package uistream

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"net/http"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type hostileEncoder struct {
	startErr, encodeErr, finishErr error
	panicEncode                    bool
	finishCount                    int
	terminal                       error
}

func (e *hostileEncoder) Start() ([]Frame, error) {
	return []Frame{{Data: []byte("start")}}, e.startErr
}

func (e *hostileEncoder) Encode(aikit.StepEvent) ([]Frame, error) {
	if e.panicEncode {
		panic("encode panic")
	}
	return []Frame{{Data: []byte("one")}, {Data: []byte("two")}}, e.encodeErr
}

func (e *hostileEncoder) Finish(err error) ([]Frame, error) {
	e.finishCount++
	e.terminal = err
	return []Frame{{Data: []byte("finish")}}, e.finishErr
}

func hostileProtocol(e *hostileEncoder) Protocol {
	return Protocol{NewEncoder: func(Options) Encoder { return e }, Framer: SSEFramer{}}
}

func TestPipeFinishesOnceAcrossFailureModes(t *testing.T) {
	want := errors.New("failure")
	for name, configure := range map[string]func(*hostileEncoder) iter.Seq2[aikit.StepEvent, error]{
		"start error":  func(e *hostileEncoder) iter.Seq2[aikit.StepEvent, error] { e.startErr = want; return nil },
		"encode error": func(e *hostileEncoder) iter.Seq2[aikit.StepEvent, error] { e.encodeErr = want; return oneEvent() },
		"encode panic": func(e *hostileEncoder) iter.Seq2[aikit.StepEvent, error] { e.panicEncode = true; return oneEvent() },
		"producer panic": func(*hostileEncoder) iter.Seq2[aikit.StepEvent, error] {
			return func(func(aikit.StepEvent, error) bool) { panic("producer panic") }
		},
		"iterator error": func(*hostileEncoder) iter.Seq2[aikit.StepEvent, error] {
			return func(yield func(aikit.StepEvent, error) bool) { yield(aikit.StepEvent{}, want) }
		},
	} {
		t.Run(name, func(t *testing.T) {
			encoder := new(hostileEncoder)
			events := configure(encoder)
			if err := Pipe(
				context.Background(),
				&bytes.Buffer{},
				events,
				hostileProtocol(encoder),
				Options{},
			); err == nil {
				t.Fatal("Pipe succeeded, want error")
			}
			if encoder.finishCount != 1 {
				t.Fatalf("Finish called %d times", encoder.finishCount)
			}
		})
	}
}

func TestPipePassesOriginalTypedErrorToFinish(t *testing.T) {
	funcError := &typedErrorForPipe{marker: "original"}
	encoder := new(hostileEncoder)
	events := iter.Seq2[aikit.StepEvent, error](func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{}, funcError)
	})
	_ = Pipe(context.Background(), &bytes.Buffer{}, events, hostileProtocol(encoder), Options{})
	var got *typedErrorForPipe
	if !errors.As(encoder.terminal, &got) || got != funcError {
		t.Fatalf("Finish received %T %v, want original typed error", encoder.terminal, encoder.terminal)
	}
}

func (e *typedErrorForPipe) Error() string { return e.marker }

type typedErrorForPipe struct{ marker string }

func TestPipeContextCancellationReachesFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	encoder := new(hostileEncoder)
	err := Pipe(ctx, &bytes.Buffer{}, nil, hostileProtocol(encoder), Options{})
	if !errors.Is(err, context.Canceled) || !errors.Is(encoder.terminal, context.Canceled) || encoder.finishCount != 1 {
		t.Fatalf("error=%v terminal=%v finish=%d", err, encoder.terminal, encoder.finishCount)
	}
}

func TestPipeRejectsNilEncoder(t *testing.T) {
	protocol := Protocol{NewEncoder: func(Options) Encoder { return nil }, Framer: SSEFramer{}}
	if err := Pipe(context.Background(), &bytes.Buffer{}, nil, protocol, Options{}); err == nil {
		t.Fatal("Pipe succeeded with nil encoder")
	}
}

type failingWriter struct{ writes int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("write failed")
}

func TestPipeReportsFirstWriteErrorOnce(t *testing.T) {
	writer := new(failingWriter)
	encoder := new(hostileEncoder)
	callbackCount := 0
	err := Pipe(
		context.Background(),
		writer,
		oneEvent(),
		hostileProtocol(encoder),
		Options{OnWriteError: func(error) { callbackCount++ }},
	)
	if err == nil || callbackCount != 1 || encoder.finishCount != 1 {
		t.Fatalf("error=%v callbacks=%d finish=%d writes=%d", err, callbackCount, encoder.finishCount, writer.writes)
	}
}

func TestPipeContainsWriteErrorCallbackPanic(t *testing.T) {
	writer := new(failingWriter)
	encoder := new(hostileEncoder)
	err := Pipe(
		context.Background(),
		writer,
		oneEvent(),
		hostileProtocol(encoder),
		Options{OnWriteError: func(error) { panic("observer panic") }},
	)
	if err == nil || encoder.finishCount != 1 {
		t.Fatalf("error=%v finish=%d", err, encoder.finishCount)
	}
}

type flushingWriter struct {
	bytes.Buffer
	flushes int
}

func (w *flushingWriter) Flush() { w.flushes++ }

var _ http.Flusher = (*flushingWriter)(nil)

func TestPipeFlushesStartBeforeReadingEmptySequence(t *testing.T) {
	writer := new(flushingWriter)
	encoder := new(hostileEncoder)
	if err := Pipe(
		context.Background(),
		writer,
		func(func(aikit.StepEvent, error) bool) {},
		hostileProtocol(encoder),
		Options{},
	); err != nil {
		t.Fatal(err)
	}
	if writer.flushes != 2 {
		t.Fatalf("flushes = %d, want start and finish", writer.flushes)
	}
}

func oneEvent() iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "x"}, nil)
	}
}
