package uistream

import (
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Frame is one protocol-encoded unit. Name maps to SSE's event field.
type Frame struct {
	Name string
	Data []byte
}

// Encoder is stateful and is used by one Pipe invocation.
type Encoder interface {
	Start() ([]Frame, error)
	Encode(aikit.StepEvent) ([]Frame, error)
	Finish(error) ([]Frame, error)
}

type Decoder interface {
	Decode(io.Reader) (Request, error)
}
type Framer interface {
	ApplyHeaders(http.Header)
	WriteFrame(io.Writer, Frame) error
}
type Protocol struct {
	NewEncoder func(Options) Encoder
	Decoder    Decoder
	Framer     Framer
}
