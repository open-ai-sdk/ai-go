# Write a UI stream adapter

Use this guide when a browser client speaks a protocol that is not AI SDK v7 or
AG-UI. An adapter translates `aikit.StepEvent` values at the edge; it must not
import `agent` or a provider package.

`uistream.Pipe` owns iterator draining, cancellation, frame flushing, panic
recovery, and calls `Finish` exactly once. Your adapter owns request parsing and
the protocol's wire format, ordering, and terminal event.

## Minimal JSON-over-SSE adapter

This deliberately small example accepts `{"prompt":"…"}` and writes one JSON
object per SSE frame. Replace its event names and request shape with those of
your client protocol.

```go
package lineproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func Protocol() uistream.Protocol {
	return uistream.Protocol{
		NewEncoder: func(uistream.Options) uistream.Encoder { return encoder{} },
		Decoder:    decoder{},
		Framer:     uistream.SSEFramer{},
	}
}

type encoder struct{}

func (encoder) Start() ([]uistream.Frame, error) { return frame("start", "") }

func (encoder) Encode(event aikit.StepEvent) ([]uistream.Frame, error) {
	switch event.Type {
	case aikit.StepEventTextDelta:
		return frame("text", event.TextDelta)
	case aikit.StepEventDone:
		// Pipe calls Finish for the one terminal event.
		return nil, nil
	default:
		return nil, fmt.Errorf("lineproto: unsupported step event %d", event.Type)
	}
}

func (encoder) Finish(terminal error) ([]uistream.Frame, error) {
	if terminal != nil {
		return frame("error", uistream.RedactStreamError(terminal))
	}
	return frame("done", "")
}

func frame(kind, value string) ([]uistream.Frame, error) {
	payload, err := json.Marshal(struct {
		Type  string `json:"type"`
		Value string `json:"value,omitempty"`
	}{Type: kind, Value: value})
	if err != nil {
		return nil, err
	}
	return []uistream.Frame{{Data: payload}}, nil
}

type decoder struct{}

func (decoder) Decode(r io.Reader) (uistream.Request, error) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&body); err != nil {
		return uistream.Request{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return uistream.Request{}, errors.New("lineproto: request must contain one JSON value")
	}
	if body.Prompt == "" {
		return uistream.Request{}, errors.New("lineproto: prompt is required")
	}
	return uistream.Request{Messages: []aikit.Message{aikit.UserMessage(body.Prompt)}}, nil
}
```

`uistream.SSEFramer` is enough for conventional SSE. Write a custom `Framer`
only when the client needs different headers or a different frame syntax.

## Serve it

Pass the protocol to the shared HTTP boundary; no custom handler loop is
needed:

```go
http.Handle("/chat", aisdkhttp.HandlerFor(lineproto.Protocol(), run))
```

Use `aisdkhttp.HandlerForRequest` instead when the decoder puts protocol-native
fields in `uistream.Request.Extra` and the run needs to read them.

## Rules to keep

- Make one encoder per request. Store no per-run state in `Protocol` globals.
- Emit the protocol's terminal success event from `Finish(nil)` and its
  redacted error event from `Finish(err)`. `StepEventError` never reaches
  `Encode`.
- Explicitly map or reject every event your application can produce. Dropping
  an event is valid only when the client protocol cannot represent it; document
  that capability gap.
- Keep decoding strict: reject malformed bodies and do not treat client-owned
  fields as trusted authorization data.
- Reuse `uistream.SSEFramer` for normal SSE. A `Frame.Name` becomes the SSE
  `event:` field; `Frame.Data` becomes `data:`.

## Minimum test

Test the public seam, not only encoder methods. Feed a short iterator through
`uistream.Pipe` and assert opening, text, and terminal frames. Add an error
case and every event ordering constraint imposed by the client protocol.

```go
func TestProtocolTerminates(t *testing.T) {
	var out bytes.Buffer
	events := func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"}, nil)
	}
	if err := uistream.Pipe(context.Background(), &out, events, Protocol(), uistream.Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"type":"done"`) {
		t.Fatalf("stream did not terminate: %s", out.String())
	}
}
```

Run `go test ./...` from `ai-go`. For an adapter aimed at a JavaScript client,
also validate its emitted frames against that client's real parser or schema.

See [Protocol extensions](/integrations/protocol-extensions) for the shared
contract, [AINode](/integrations/ui-streams) for a strict production wire
implementation, and [AG-UI](/integrations/ag-ui) for a stateful adapter.
