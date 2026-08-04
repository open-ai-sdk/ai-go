package uistream

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type testEncoder struct {
	started, finished int
	terminal          error
	gotError          bool
}

func (e *testEncoder) Start() ([]Frame, error) {
	e.started++
	return []Frame{{Data: []byte("start")}}, nil
}
func (e *testEncoder) Encode(v aikit.StepEvent) ([]Frame, error) {
	if v.Type == aikit.StepEventError {
		e.gotError = true
	}
	return []Frame{{Data: []byte("event")}}, nil
}
func (e *testEncoder) Finish(err error) ([]Frame, error) {
	e.finished++
	e.terminal = err
	return []Frame{{Data: []byte("finish")}}, nil
}
func testProtocol(e *testEncoder) Protocol {
	return Protocol{NewEncoder: func(Options) Encoder { return e }, Framer: SSEFramer{}}
}
func TestPipeNormalizesErrorEventAndFinishes(t *testing.T) {
	e := new(testEncoder)
	var b bytes.Buffer
	want := errors.New("provider failure")
	events := func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{Type: aikit.StepEventError, Error: want}, nil)
	}
	if err := Pipe(context.Background(), &b, events, testProtocol(e), Options{}); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	if e.started != 1 || e.finished != 1 || e.gotError {
		t.Fatalf("encoder state %#v", e)
	}
	if !errors.Is(e.terminal, want) {
		t.Fatalf("finish error = %v", e.terminal)
	}
}
func TestPipeIteratorErrorMatchesEventError(t *testing.T) {
	e := new(testEncoder)
	want := errors.New("failure")
	events := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) { yield(aikit.StepEvent{}, want) })
	if err := Pipe(context.Background(), ioDiscard{}, events, testProtocol(e), Options{}); !errors.Is(err, want) || !errors.Is(e.terminal, want) {
		t.Fatalf("errors %v %v", err, e.terminal)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
