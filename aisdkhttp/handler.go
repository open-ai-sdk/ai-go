package aisdkhttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdk"
)

// RunFunc starts an agent run for the messages decoded from a v7 chat request.
// Returning an error reports a pre-stream failure as HTTP 500. Errors emitted
// through the returned sequence are redacted and encoded as error chunks.
type RunFunc func(
	ctx context.Context,
	messages []aikit.Message,
) (iter.Seq2[aikit.StepEvent, error], error)

// Handler returns an http.Handler for v7 chat POSTs.
func Handler(run RunFunc) http.Handler {
	if run == nil {
		panic("aisdkhttp: nil RunFunc")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		defer func() { _ = r.Body.Close() }()

		envelope, err := decodeEnvelope(r.Body)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, invalidRequestMessage)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		events, err := run(ctx, aisdk.ToAIMessages(envelope.Messages))
		if err != nil || events == nil {
			writeHTTPError(w, http.StatusInternalServerError, streamErrorMessage)
			return
		}

		messageID := ""
		if envelope.Trigger == "regenerate-message" {
			messageID = envelope.MessageID
		}
		if messageID == "" {
			messageID = newMessageID()
		}
		chunks := aisdk.NewChunkProducer(messageID).Produce(eventChannel(ctx, events)).Chunks
		writer := newSSEWriter(w, cancel)
		if err := aisdk.WriteSSEStream(writer, chunks); err != nil {
			cancel()
			return
		}
	})
}

func eventChannel(
	ctx context.Context,
	events iter.Seq2[aikit.StepEvent, error],
) <-chan aikit.StepEvent {
	stream := make(chan aikit.StepEvent)
	go func() {
		defer close(stream)
		for event, err := range events {
			if err != nil {
				select {
				case stream <- aikit.StepEvent{Type: aikit.StepEventError, Error: err}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case stream <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return stream
}

func decodeEnvelope(body io.Reader) (aisdk.ChatRequestEnvelope, error) {
	var envelope *aisdk.ChatRequestEnvelope
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&envelope); err != nil {
		return aisdk.ChatRequestEnvelope{}, err
	}
	if envelope == nil {
		return aisdk.ChatRequestEnvelope{}, errors.New("null request envelope")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return aisdk.ChatRequestEnvelope{}, errors.New("multiple JSON values")
		}
		return aisdk.ChatRequestEnvelope{}, err
	}
	return *envelope, nil
}

func newMessageID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "msg"
	}
	return "msg_" + hex.EncodeToString(bytes[:])
}
