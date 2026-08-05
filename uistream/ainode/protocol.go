package ainode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// Protocol supplies the AI SDK v7 encoder, decoder, and SSE framer.
func Protocol() uistream.Protocol {
	return uistream.Protocol{
		NewEncoder: func(options uistream.Options) uistream.Encoder {
			return &encoder{producer: NewChunkProducer(options.MessageID)}
		},
		Decoder: decoder{},
		Framer:  framer{},
	}
}

type framer struct{}

func (framer) ApplyHeaders(header http.Header) {
	(uistream.SSEFramer{}).ApplyHeaders(header)
	header.Set("x-vercel-ai-ui-message-stream", "v1")
	header.Set("x-accel-buffering", "no")
}

func (framer) WriteFrame(w io.Writer, frame uistream.Frame) error {
	return (uistream.SSEFramer{}).WriteFrame(w, frame)
}

type encoder struct {
	producer *ChunkProducer
}

func (e *encoder) Start() ([]uistream.Frame, error) {
	return e.frames([]Chunk{{Type: ChunkStart, Fields: map[string]any{"messageId": e.producer.msgID}}})
}

func (e *encoder) Encode(event aikit.StepEvent) ([]uistream.Frame, error) {
	chunks, _ := e.producer.translateEvent(event)
	return e.frames(chunks)
}

func (e *encoder) Finish(terminal error) ([]uistream.Frame, error) {
	defer func() {
		for _, violation := range e.producer.checker.Finalize() {
			reportInvariant(e.producer.logger, e.producer.reporter, violation)
		}
	}()
	if terminal == nil {
		return nil, nil
	}
	return e.frames(e.producer.chunksError(terminal))
}

func (e *encoder) frames(chunks []Chunk) ([]uistream.Frame, error) {
	frames := make([]uistream.Frame, 0, len(chunks)+1)
	for _, chunk := range chunks {
		for _, violation := range e.producer.checker.Observe(chunk) {
			reportInvariant(e.producer.logger, e.producer.reporter, violation)
		}
		data, err := encodeChunkPayload(chunk, false)
		if err != nil {
			return nil, err
		}
		frames = append(frames, uistream.Frame{Data: data})
		if chunk.Type == ChunkFinish {
			frames = append(frames, uistream.Frame{Data: []byte("[DONE]")})
		}
	}
	return frames, nil
}

type decoder struct{}

func (decoder) Decode(reader io.Reader) (uistream.Request, error) {
	var envelope *ChatRequestEnvelope
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&envelope); err != nil {
		return uistream.Request{}, err
	}
	if envelope == nil {
		return uistream.Request{}, errors.New("null request envelope")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return uistream.Request{}, errors.New("multiple JSON values")
		}
		return uistream.Request{}, err
	}

	messageID := ""
	if envelope.Trigger == "regenerate-message" {
		messageID = envelope.MessageID
	}
	if messageID == "" {
		messageID = newMessageID()
	}
	return uistream.Request{
		Messages:  ToAIMessages(envelope.Messages),
		MessageID: messageID,
		ID:        envelope.ID,
		Body:      envelope.Body,
		Metadata:  envelope.Metadata,
		Trigger:   envelope.Trigger,
	}, nil
}

func newMessageID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "msg"
	}
	return "msg_" + hex.EncodeToString(value[:])
}
