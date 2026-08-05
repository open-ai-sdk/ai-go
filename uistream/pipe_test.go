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
	want := errors.New("failure")
	iteratorEvents := iter.Seq2[aikit.StepEvent, error](
		func(yield func(aikit.StepEvent, error) bool) { yield(aikit.StepEvent{}, want) },
	)
	eventEvents := iter.Seq2[aikit.StepEvent, error](
		func(yield func(aikit.StepEvent, error) bool) {
			yield(aikit.StepEvent{Type: aikit.StepEventError, Error: want}, nil)
		},
	)

	var outputs [2]bytes.Buffer
	for index, events := range []iter.Seq2[aikit.StepEvent, error]{iteratorEvents, eventEvents} {
		encoder := new(testEncoder)
		if err := Pipe(
			context.Background(),
			&outputs[index],
			events,
			testProtocol(encoder),
			Options{},
		); !errors.Is(err, want) ||
			!errors.Is(encoder.terminal, want) {
			t.Fatalf("case %d errors %v %v", index, err, encoder.terminal)
		}
	}
	if outputs[0].String() != outputs[1].String() {
		t.Fatalf("normalized bytes differ: iterator=%q event=%q", outputs[0].String(), outputs[1].String())
	}
}
