package aisdkhttp

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type flushRecorder struct {
	*httptest.ResponseRecorder
	mu      sync.Mutex
	flushes int
	flushed chan struct{}
}

func (r *flushRecorder) Flush() {
	r.mu.Lock()
	r.flushes++
	r.mu.Unlock()
	r.ResponseRecorder.Flush()
	select {
	case r.flushed <- struct{}{}:
	default:
	}
}

func TestHandlerFlushesBeforeStreamCompletion(t *testing.T) {
	events := make(chan aikit.StepEvent)
	run := func(context.Context, []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
		return sequenceFromChannel(events), nil
	}
	recorder := &flushRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan struct{}, 1),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Handler(run).ServeHTTP(
			recorder,
			httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"messages":[]}`)),
		)
	}()

	select {
	case <-recorder.flushed:
		// The start frame was visible while the agent stream was still open.
	case <-time.After(time.Second):
		t.Fatal("first SSE frame was not flushed promptly")
	}

	events <- aikit.StepEvent{Type: aikit.StepEventDone}
	close(events)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after the event stream closed")
	}

	recorder.mu.Lock()
	flushes := recorder.flushes
	recorder.mu.Unlock()
	if flushes < 3 {
		t.Fatalf("flush count = %d, want at least 3 (start, finish, done)", flushes)
	}
}
