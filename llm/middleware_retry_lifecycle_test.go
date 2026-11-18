package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type retryLifecycleModel struct {
	calls  atomic.Int32
	stream func(context.Context, int) (<-chan aikit.StreamEvent, error)
}

func (*retryLifecycleModel) ModelID() string { return "retry-lifecycle-test" }

func (m *retryLifecycleModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	return m.stream(ctx, int(m.calls.Add(1)))
}

func retryLifecycleContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func retryLifecycleEvents(events ...aikit.StreamEvent) <-chan aikit.StreamEvent {
	out := make(chan aikit.StreamEvent, len(events))
	for _, ev := range events {
		out <- ev
	}
	close(out)
	return out
}

func retryLifecycleSend(ctx context.Context, out chan<- aikit.StreamEvent, ev aikit.StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func retryLifecycleNext(t *testing.T, ctx context.Context, events <-chan aikit.StreamEvent) aikit.StreamEvent {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("stream closed before the expected event")
		}
		return ev
	case <-ctx.Done():
		t.Fatalf("waiting for event: %v", ctx.Err())
		return aikit.StreamEvent{}
	}
}

func retryLifecycleCollect(t *testing.T, ctx context.Context, events <-chan aikit.StreamEvent) []aikit.StreamEvent {
	t.Helper()
	var got []aikit.StreamEvent
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-ctx.Done():
			t.Fatalf("waiting for stream closure: %v; events=%+v", ctx.Err(), got)
			return nil
		}
	}
}

func retryLifecycleAwait(t *testing.T, ctx context.Context, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("waiting for producer: %v", ctx.Err())
	}
}

func retryLifecycleStopped(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	default:
		t.Fatal("stream closed or retried before its producer stopped")
	}
}

func retryLifecycleWant(t *testing.T, got []aikit.StreamEvent, want ...aikit.StreamEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %+v; want %+v", got, want)
	}
	for i, actual := range got {
		expected := want[i]
		if !errors.Is(actual.Error, expected.Error) {
			t.Fatalf("event %d error = %v; want %v", i, actual.Error, expected.Error)
		}
		actual.Error, expected.Error = nil, nil
		actualJSON, actualErr := json.Marshal(actual)
		expectedJSON, expectedErr := json.Marshal(expected)
		if actualErr != nil || expectedErr != nil || string(actualJSON) != string(expectedJSON) {
			t.Fatalf("event %d = %+v; want %+v", i, got[i], want[i])
		}
	}
}

func retryLifecycleCommitText(ev aikit.StreamEvent) bool {
	return ev.Type == aikit.StreamEventTextDelta && ev.TextDelta != ""
}

func TestWithRetry_LifecycleReasoningBeforeRetry(t *testing.T) {
	ctx := retryLifecycleContext(t)
	cause := aikit.NewAPIError("test", 503, nil)
	oldID, newID := "reasoning-old", "reasoning-new"
	oldReasoning := aikit.StreamEvent{
		Type:        aikit.StreamEventReasoningDelta,
		TextDelta:   "first thought",
		ReasoningID: &oldID,
	}
	oldSummary := aikit.StreamEvent{
		Type:             aikit.StreamEventReasoningSummary,
		ReasoningSummary: "first summary",
		ReasoningID:      &oldID,
	}
	newReasoning := aikit.StreamEvent{
		Type:        aikit.StreamEventReasoningDelta,
		TextDelta:   "second thought",
		ReasoningID: &newID,
	}
	releaseFailure := make(chan struct{})
	oldStopped := make(chan struct{})
	type notice struct {
		attempt int
		cause   error
	}
	notices := make(chan notice, 1)
	m := &retryLifecycleModel{}
	m.stream = func(attemptCtx context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
		if attempt != 1 {
			select {
			case <-oldStopped:
			default:
				return nil, errors.New("next attempt started before the old producer stopped")
			}
			return retryLifecycleEvents(newReasoning, mwText("answer"), mwFinish()), nil
		}
		events := make(chan aikit.StreamEvent)
		go func() {
			defer close(events)
			defer close(oldStopped)
			if !retryLifecycleSend(attemptCtx, events, oldReasoning) ||
				!retryLifecycleSend(attemptCtx, events, oldSummary) ||
				!retryLifecycleSend(attemptCtx, events, mwText("")) {
				return
			}
			select {
			case <-releaseFailure:
			case <-attemptCtx.Done():
				return
			}
			if !retryLifecycleSend(attemptCtx, events, mwFail(cause)) {
				return
			}
			<-attemptCtx.Done()
			// A final in-flight event must be drained, never exposed after the
			// retry boundary. The parent context bounds a broken join in tests.
			retryLifecycleSend(ctx, events, mwText("stale attempt output"))
		}()
		return events, nil
	}
	out, err := llm.WithRetry(m, llm.RetryOptions{
		MaxRetries: 1, CommitOn: retryLifecycleCommitText, EmitRetryEvents: true,
		OnRetry: func(attempt int, cause error) { notices <- notice{attempt, cause} },
	}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	got := []aikit.StreamEvent{
		retryLifecycleNext(t, ctx, out), retryLifecycleNext(t, ctx, out), retryLifecycleNext(t, ctx, out),
	}
	retryLifecycleWant(t, got, oldReasoning, oldSummary, mwText(""))
	// Receiving these events before releasing the failure proves they are not
	// buffered until nonempty text or discarded with the failed attempt.
	close(releaseFailure)
	boundary := retryLifecycleNext(t, ctx, out)
	retryLifecycleStopped(t, oldStopped)
	got = append(got, boundary)
	got = append(got, retryLifecycleCollect(t, ctx, out)...)
	retryLifecycleWant(t, got, oldReasoning, oldSummary, mwText(""),
		aikit.StreamEvent{Type: aikit.StreamEventRetry, Error: cause}, newReasoning, mwText("answer"), mwFinish())
	if m.calls.Load() != 2 {
		t.Fatalf("calls = %d; want 2", m.calls.Load())
	}
	select {
	case got := <-notices:
		if got.attempt != 1 || !errors.Is(got.cause, cause) {
			t.Fatalf("OnRetry = %+v", got)
		}
	default:
		t.Fatal("OnRetry was not called")
	}
}

func TestWithRetry_LifecycleNeverRetriesAfterCommit(t *testing.T) {
	cause := aikit.NewAPIError("test", 503, nil)
	for _, tc := range []struct {
		name     string
		commitOn func(aikit.StreamEvent) bool
		prefix   []aikit.StreamEvent
	}{
		{"nonempty text", retryLifecycleCommitText, []aikit.StreamEvent{
			{Type: aikit.StreamEventReasoningDelta, TextDelta: "thought"}, mwText(""), mwText("partial"),
		}},
		{"finish", func(aikit.StreamEvent) bool { return false }, []aikit.StreamEvent{mwFinish()}},
		{"default reasoning", nil, []aikit.StreamEvent{{Type: aikit.StreamEventReasoningDelta, TextDelta: "thought"}}},
		{"custom usage", func(ev aikit.StreamEvent) bool { return ev.Type == aikit.StreamEventUsage }, []aikit.StreamEvent{mwUsage()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := retryLifecycleContext(t)
			terminal := aikit.StreamEvent{Type: aikit.StreamEventError, Error: cause, MessageID: "failed-message"}
			want := append(append([]aikit.StreamEvent(nil), tc.prefix...), terminal)
			m := &retryLifecycleModel{stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) {
				return retryLifecycleEvents(want...), nil
			}}
			var retries atomic.Int32
			out, err := llm.WithRetry(m, llm.RetryOptions{
				MaxRetries: 3, CommitOn: tc.commitOn, EmitRetryEvents: true,
				OnRetry: func(int, error) { retries.Add(1) },
			}).Stream(ctx, llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			retryLifecycleWant(t, retryLifecycleCollect(t, ctx, out), want...)
			if m.calls.Load() != 1 || retries.Load() != 0 {
				t.Fatalf("calls=%d retries=%d", m.calls.Load(), retries.Load())
			}
		})
	}
}

func TestWithRetry_CustomCommitWithoutLifecycleKeepsBuffering(t *testing.T) {
	ctx := retryLifecycleContext(t)
	cause := aikit.NewAPIError("test", 503, nil)
	m := &retryLifecycleModel{stream: func(_ context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
		if attempt == 1 {
			return retryLifecycleEvents(
				mwUsage(),
				aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "discarded"},
				mwText(""),
				mwFail(cause),
			), nil
		}
		return retryLifecycleEvents(
			aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "kept"},
			mwText("answer"),
			mwFinish(),
		), nil
	}}
	out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1, CommitOn: retryLifecycleCommitText}).
		Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	retryLifecycleWant(t, retryLifecycleCollect(t, ctx, out),
		aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "kept"}, mwText("answer"), mwFinish())
	if m.calls.Load() != 2 {
		t.Fatalf("calls = %d; want 2", m.calls.Load())
	}
}

func TestWithRetry_LifecycleStartupBoundariesPrecedeHooksAndAttempts(t *testing.T) {
	ctx := retryLifecycleContext(t)
	causes := []error{aikit.NewAPIError("test", 503, nil), aikit.NewAPIError("test", 429, nil)}
	m := &retryLifecycleModel{stream: func(_ context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
		if attempt <= len(causes) {
			return nil, causes[attempt-1]
		}
		return retryLifecycleEvents(mwText("answer"), mwFinish()), nil
	}}
	type notice struct {
		attempt int
		cause   error
	}
	hook := make(chan notice)
	release := make(chan struct{})
	out, err := llm.WithRetry(m, llm.RetryOptions{
		MaxRetries: 2, EmitRetryEvents: true,
		OnRetry: func(attempt int, cause error) {
			select {
			case hook <- notice{attempt, cause}:
			case <-ctx.Done():
				return
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
		},
	}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for i, cause := range causes {
		ev := retryLifecycleNext(t, ctx, out)
		retryLifecycleWant(t, []aikit.StreamEvent{ev}, aikit.StreamEvent{Type: aikit.StreamEventRetry, Error: cause})
		select {
		case got := <-hook:
			if got.attempt != i+1 || !errors.Is(got.cause, cause) {
				t.Fatalf("OnRetry = %+v", got)
			}
		case <-ctx.Done():
			t.Fatal("OnRetry did not follow its boundary")
		}
		if m.calls.Load() != int32(i+1) {
			t.Fatalf("next attempt ran before OnRetry/wait: calls=%d", m.calls.Load())
		}
		select {
		case release <- struct{}{}:
		case <-ctx.Done():
			t.Fatal("OnRetry did not finish")
		}
	}
	retryLifecycleWant(t, retryLifecycleCollect(t, ctx, out), mwText("answer"), mwFinish())
	if m.calls.Load() != 3 {
		t.Fatalf("calls = %d; want 3", m.calls.Load())
	}
}

func TestWithRetry_LifecycleProviderRejectionOverridesClassifier(t *testing.T) {
	for _, code := range []int{0, 400, 401, 403, 404, 422} {
		for _, startup := range []bool{false, true} {
			t.Run(fmt.Sprintf("status_%d/startup_%t", code, startup), func(t *testing.T) {
				ctx := retryLifecycleContext(t)
				apiErr := aikit.NewAPIError("test", code, nil)
				if code == 0 {
					apiErr.Code = "invalid_request_error"
				}
				cause := fmt.Errorf("wrapped rejection: %w", apiErr)
				m := &retryLifecycleModel{stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) {
					if startup {
						return nil, cause
					}
					return retryLifecycleEvents(mwFail(cause)), nil
				}}
				var classified, retried atomic.Int32
				out, err := llm.WithRetry(m, llm.RetryOptions{
					MaxRetries: 2, CommitOn: retryLifecycleCommitText, EmitRetryEvents: true,
					Retryable: func(error) bool { classified.Add(1); return true },
					OnRetry:   func(int, error) { retried.Add(1) },
				}).Stream(ctx, llm.Request{})
				if err != nil {
					t.Fatal(err)
				}
				retryLifecycleWant(t, retryLifecycleCollect(t, ctx, out), mwFail(cause))
				if m.calls.Load() != 1 || classified.Load() != 0 || retried.Load() != 0 {
					t.Fatalf("calls=%d classifier=%d retries=%d", m.calls.Load(), classified.Load(), retried.Load())
				}
			})
		}
	}
}

func TestWithRetry_LifecycleExhaustionPropagatesFinalError(t *testing.T) {
	for _, finalStartup := range []bool{false, true} {
		t.Run(fmt.Sprintf("startup_%t", finalStartup), func(t *testing.T) {
			ctx := retryLifecycleContext(t)
			first := aikit.NewAPIError("test", 503, nil)
			last := aikit.NewAPIError("test", 429, nil)
			reasoning := aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "unfinished"}
			m := &retryLifecycleModel{stream: func(_ context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
				if attempt == 1 {
					return retryLifecycleEvents(reasoning, mwFail(first)), nil
				}
				if finalStartup {
					return nil, last
				}
				return retryLifecycleEvents(mwFail(last)), nil
			}}
			out, err := llm.WithRetry(m, llm.RetryOptions{
				MaxRetries: 1, CommitOn: retryLifecycleCommitText, EmitRetryEvents: true,
			}).Stream(ctx, llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			retryLifecycleWant(t, retryLifecycleCollect(t, ctx, out), reasoning,
				aikit.StreamEvent{Type: aikit.StreamEventRetry, Error: first}, mwFail(last))
			if m.calls.Load() != 2 {
				t.Fatalf("calls = %d; want 2", m.calls.Load())
			}
		})
	}
}

func TestWithRetry_LifecycleMalformedErrorIsTerminal(t *testing.T) {
	ctx := retryLifecycleContext(t)
	m := &retryLifecycleModel{stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) {
		return retryLifecycleEvents(aikit.StreamEvent{Type: aikit.StreamEventError}), nil
	}}
	out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1, EmitRetryEvents: true}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	got := retryLifecycleCollect(t, ctx, out)
	if len(got) != 1 || got[0].Type != aikit.StreamEventError || got[0].Error == nil || m.calls.Load() != 1 {
		t.Fatalf("events=%+v calls=%d", got, m.calls.Load())
	}
}

func TestWithRetry_LifecycleCancellationJoinsProducer(t *testing.T) {
	for _, phase := range []string{"awaiting event", "blocked forwarding", "after commit"} {
		t.Run(phase, func(t *testing.T) {
			deadline := retryLifecycleContext(t)
			ctx, cancel := context.WithCancel(deadline)
			defer cancel()
			ready, stopped := make(chan struct{}), make(chan struct{})
			m := &retryLifecycleModel{
				stream: func(attemptCtx context.Context, _ int) (<-chan aikit.StreamEvent, error) {
					events := make(chan aikit.StreamEvent)
					go func() {
						defer close(events)
						defer close(stopped)
						if phase != "awaiting event" {
							ev := aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "thought"}
							if phase == "after commit" {
								ev = mwText("partial")
							}
							if !retryLifecycleSend(attemptCtx, events, ev) {
								return
							}
						}
						close(ready)
						<-attemptCtx.Done()
					}()
					return events, nil
				},
			}
			out, err := llm.WithRetry(m, llm.RetryOptions{
				MaxRetries: 2, CommitOn: retryLifecycleCommitText, EmitRetryEvents: true,
			}).Stream(ctx, llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if phase == "after commit" {
				retryLifecycleWant(t, []aikit.StreamEvent{retryLifecycleNext(t, deadline, out)}, mwText("partial"))
			}
			retryLifecycleAwait(t, deadline, ready)
			cancel()
			for _, ev := range retryLifecycleCollect(t, deadline, out) {
				if ev.Type == aikit.StreamEventRetry || ev.Type == aikit.StreamEventError {
					t.Fatalf("cancellation produced an error or retry: %+v", ev)
				}
			}
			retryLifecycleStopped(t, stopped)
			if m.calls.Load() != 1 {
				t.Fatalf("calls = %d; want 1", m.calls.Load())
			}
		})
	}
}

func TestWithRetry_LifecycleCancellationBeforeBoundarySkipsHook(t *testing.T) {
	deadline := retryLifecycleContext(t)
	ctx, cancel := context.WithCancel(deadline)
	defer cancel()
	started := make(chan struct{})
	m := &retryLifecycleModel{stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) {
		close(started)
		return nil, aikit.NewAPIError("test", 503, nil)
	}}
	var retries atomic.Int32
	out, err := llm.WithRetry(m, llm.RetryOptions{
		MaxRetries: 2, EmitRetryEvents: true,
		OnRetry: func(int, error) { retries.Add(1) },
	}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	retryLifecycleAwait(t, deadline, started)
	cancel()
	// Do not receive the pending boundary: cancellation must release its send
	// without invoking OnRetry or starting another attempt.
	retryLifecycleAwait(t, deadline, ctx.Done())
	for range retryLifecycleCollect(t, deadline, out) {
		// A send racing cancellation may win once the consumer resumes.
	}
	if retries.Load() != 0 || m.calls.Load() != 1 {
		t.Fatalf("calls=%d retries=%d", m.calls.Load(), retries.Load())
	}
}

func TestWithRetry_LifecycleCancellationDuringBackoff(t *testing.T) {
	deadline := retryLifecycleContext(t)
	ctx, cancel := context.WithCancel(deadline)
	defer cancel()
	cause := aikit.NewAPIError("test", 503, nil)
	m := &retryLifecycleModel{
		stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) { return nil, cause },
	}
	hook := make(chan struct{})
	out, err := llm.WithRetry(m, llm.RetryOptions{
		MaxRetries: 2, InitialDelay: time.Hour, EmitRetryEvents: true,
		OnRetry: func(int, error) { close(hook) },
	}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	retryLifecycleWant(t, []aikit.StreamEvent{retryLifecycleNext(t, deadline, out)},
		aikit.StreamEvent{Type: aikit.StreamEventRetry, Error: cause})
	retryLifecycleAwait(t, deadline, hook)
	cancel()
	if got := retryLifecycleCollect(t, deadline, out); len(got) != 0 || m.calls.Load() != 1 {
		t.Fatalf("events=%+v calls=%d", got, m.calls.Load())
	}
}

func TestWithRetry_LifecycleCancellationDuringStartup(t *testing.T) {
	deadline := retryLifecycleContext(t)
	ctx, cancel := context.WithCancel(deadline)
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	m := &retryLifecycleModel{stream: func(attemptCtx context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
		if attempt == 1 {
			return nil, aikit.NewAPIError("test", 503, nil)
		}
		close(started)
		<-attemptCtx.Done()
		close(stopped)
		return nil, attemptCtx.Err()
	}}
	out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 3, EmitRetryEvents: true}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if ev := retryLifecycleNext(t, deadline, out); ev.Type != aikit.StreamEventRetry {
		t.Fatalf("first event = %+v; want retry", ev)
	}
	retryLifecycleAwait(t, deadline, started)
	cancel()
	if got := retryLifecycleCollect(t, deadline, out); len(got) != 0 || m.calls.Load() != 2 {
		t.Fatalf("events=%+v calls=%d", got, m.calls.Load())
	}
	retryLifecycleStopped(t, stopped)
}

func TestWithRetry_LifecycleAlreadyCancelledDoesNotStartProducer(t *testing.T) {
	deadline := retryLifecycleContext(t)
	ctx, cancel := context.WithCancel(deadline)
	cancel()
	m := &retryLifecycleModel{stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) {
		return nil, errors.New("must not start")
	}}
	out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1, EmitRetryEvents: true}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := retryLifecycleCollect(t, deadline, out); len(got) != 0 || m.calls.Load() != 0 {
		t.Fatalf("events=%+v calls=%d", got, m.calls.Load())
	}
}

func TestWithRetry_DefaultStillBuffersAndCommitsReasoning(t *testing.T) {
	for _, reasoningCommits := range []bool{false, true} {
		t.Run(fmt.Sprintf("reasoning_%t", reasoningCommits), func(t *testing.T) {
			ctx := retryLifecycleContext(t)
			cause := aikit.NewAPIError("test", 503, nil)
			first := mwUsage()
			if reasoningCommits {
				first = aikit.StreamEvent{Type: aikit.StreamEventReasoningDelta, TextDelta: "thought"}
			}
			m := &retryLifecycleModel{stream: func(_ context.Context, attempt int) (<-chan aikit.StreamEvent, error) {
				if attempt == 1 {
					return retryLifecycleEvents(first, mwFail(cause)), nil
				}
				return retryLifecycleEvents(mwText("answer"), mwFinish()), nil
			}}
			out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(ctx, llm.Request{})
			if err != nil {
				t.Fatal(err)
			}
			got := retryLifecycleCollect(t, ctx, out)
			if reasoningCommits {
				retryLifecycleWant(t, got, first, mwFail(cause))
				if m.calls.Load() != 1 {
					t.Fatalf("calls = %d; want 1", m.calls.Load())
				}
			} else {
				retryLifecycleWant(t, got, mwText("answer"), mwFinish())
				if m.calls.Load() != 2 {
					t.Fatalf("calls = %d; want 2", m.calls.Load())
				}
			}
		})
	}
}

func TestWithRetry_DefaultStartupFailureRemainsSynchronous(t *testing.T) {
	ctx := retryLifecycleContext(t)
	cause := aikit.NewAPIError("test", 503, nil)
	m := &retryLifecycleModel{
		stream: func(context.Context, int) (<-chan aikit.StreamEvent, error) { return nil, cause },
	}
	out, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(ctx, llm.Request{})
	if out != nil || !errors.Is(err, cause) || m.calls.Load() != 2 {
		t.Fatalf("channel=%v err=%v calls=%d", out, err, m.calls.Load())
	}
}

func TestWithRetry_DisabledIgnoresLifecycleOptions(t *testing.T) {
	m := &retryLifecycleModel{}
	for _, maxRetries := range []int{0, -1} {
		if got := llm.WithRetry(m, llm.RetryOptions{
			MaxRetries: maxRetries, CommitOn: retryLifecycleCommitText, EmitRetryEvents: true,
		}); got != m {
			t.Fatal("disabled retry must return the original model")
		}
	}
}
