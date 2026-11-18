package llm_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// mwScriptedModel plays one mwScript per Stream call.
type mwScriptedModel struct {
	scripts []mwScript
	calls   atomic.Int32
	seen    []context.Context
}

type mwScript struct {
	err    error
	events []aikit.StreamEvent
	// hang keeps the channel open after events until ctx is cancelled.
	hang bool
	// gap is slept before every event.
	gap time.Duration
}

func (m *mwScriptedModel) ModelID() string { return "scripted" }

func (m *mwScriptedModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	n := int(m.calls.Add(1)) - 1
	m.seen = append(m.seen, ctx)
	s := m.scripts[min(n, len(m.scripts)-1)]
	if s.err != nil {
		return nil, s.err
	}
	ch := make(chan aikit.StreamEvent, 8)
	go func() {
		defer close(ch)
		for _, ev := range s.events {
			select {
			case <-time.After(s.gap):
			case <-ctx.Done():
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		if s.hang {
			<-ctx.Done()
		}
	}()
	return ch, nil
}

type mwCapableModel struct{ mwScriptedModel }

func (*mwCapableModel) NativeSchemaSupport() llm.NativeSchemaSupport {
	return llm.NativeSchemaSuppressesTools
}

func mwText(s string) aikit.StreamEvent {
	return aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: s}
}

func mwFail(err error) aikit.StreamEvent {
	return aikit.StreamEvent{Type: aikit.StreamEventError, Error: err}
}

func mwUsage() aikit.StreamEvent {
	return aikit.StreamEvent{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 1}}
}

func mwFinish() aikit.StreamEvent {
	return aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
}

func mwDrain(ch <-chan aikit.StreamEvent) []aikit.StreamEvent {
	var out []aikit.StreamEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func mwLastError(events []aikit.StreamEvent) error {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == aikit.StreamEventError {
			return events[i].Error
		}
	}
	return nil
}

func mwTexts(events []aikit.StreamEvent) (s string) {
	for _, e := range events {
		if e.Type == aikit.StreamEventTextDelta {
			s += e.TextDelta
		}
	}
	return s
}

// ---- stall ----

func TestWithStallTimeout_DisabledReturnsModel(t *testing.T) {
	m := &mwScriptedModel{}
	if llm.WithStallTimeout(m, 0) != llm.Model(m) {
		t.Fatal("zero duration must not wrap")
	}
}

func TestWithStallTimeout_TimesOutOnSilence(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{hang: true}}}
	w := llm.WithStallTimeout(m, 30*time.Millisecond)
	ch, err := w.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	events := mwDrain(ch)
	var stall *llm.StallTimeoutError
	if !errors.As(mwLastError(events), &stall) || stall.After != 30*time.Millisecond {
		t.Fatalf("events = %+v", events)
	}
	if !stall.Timeout() || stall.Temporary() || stall.Error() == "" {
		t.Fatal("stall error contract")
	}
	if m.seen[0].Err() == nil {
		t.Fatal("inner context must be cancelled on stall")
	}
}

func TestWithStallTimeout_ResetsOnlyOnEvents(t *testing.T) {
	// Events every 20ms keep a 60ms timer alive well past 60ms total.
	events := []aikit.StreamEvent{mwText("a"), mwText("b"), mwText("c"), mwText("d"), mwText("e"), mwFinish()}
	m := &mwScriptedModel{scripts: []mwScript{{events: events, gap: 20 * time.Millisecond}}}
	ch, err := llm.WithStallTimeout(m, 60*time.Millisecond).Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	got := mwDrain(ch)
	if mwLastError(got) != nil || mwTexts(got) != "abcde" {
		t.Fatalf("events = %+v", got)
	}
}

func TestWithStallTimeout_StallAfterContent(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{events: []aikit.StreamEvent{mwText("x")}, hang: true}}}
	ch, _ := llm.WithStallTimeout(m, 30*time.Millisecond).Stream(context.Background(), llm.Request{})
	got := mwDrain(ch)
	if mwTexts(got) != "x" || mwLastError(got) == nil {
		t.Fatalf("events = %+v", got)
	}
}

func TestWithStallTimeout_StreamCallItselfStalls(t *testing.T) {
	m := &mwBlockingModel{}
	_, err := llm.WithStallTimeout(m, 20*time.Millisecond).Stream(context.Background(), llm.Request{})
	var stall *llm.StallTimeoutError
	if !errors.As(err, &stall) {
		t.Fatalf("err = %v", err)
	}
}

type mwBlockingModel struct{}

func (mwBlockingModel) ModelID() string { return "blocking" }
func (mwBlockingModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestWithStallTimeout_StreamErrorPassesThrough(t *testing.T) {
	boom := errors.New("boom")
	_, err := llm.WithStallTimeout(&mwScriptedModel{scripts: []mwScript{{err: boom}}}, time.Second).
		Stream(context.Background(), llm.Request{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithStallTimeout_HonoursCallerContext(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{hang: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := llm.WithStallTimeout(m, time.Minute).Stream(ctx, llm.Request{})
	cancel()
	done := make(chan struct{})
	go func() { mwDrain(ch); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream not released after cancel")
	}
}

func TestMiddlewareKeepsIdentityAndCapability(t *testing.T) {
	plain := llm.WithStallTimeout(&mwScriptedModel{}, time.Second)
	if plain.ModelID() != "scripted" {
		t.Fatal("ModelID")
	}
	if _, ok := plain.(llm.NativeSchemaCapable); ok {
		t.Fatal("plain model must not gain NativeSchemaCapable")
	}
	if _, ok := plain.(llm.CompletionModel); ok {
		t.Fatal("middleware must not expose Complete")
	}
	capable := llm.WithRetry(&mwCapableModel{}, llm.RetryOptions{MaxRetries: 1})
	c, ok := capable.(llm.NativeSchemaCapable)
	if !ok || c.NativeSchemaSupport() != llm.NativeSchemaSuppressesTools {
		t.Fatal("capability must be forwarded")
	}
}

// ---- retry ----

func mwAPIErr(status int) error { return aikit.NewAPIError("p", status, nil) }

func TestWithRetry_DisabledReturnsModel(t *testing.T) {
	m := &mwScriptedModel{}
	if llm.WithRetry(m, llm.RetryOptions{}) != llm.Model(m) {
		t.Fatal("zero options must not wrap")
	}
}

func TestWithRetry_RetriesStreamCallError(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{
		{err: mwAPIErr(503)}, {err: mwAPIErr(429)}, {events: []aikit.StreamEvent{mwText("ok"), mwFinish()}},
	}}
	var attempts []int
	w := llm.WithRetry(
		m,
		llm.RetryOptions{MaxRetries: 2, OnRetry: func(n int, _ error) { attempts = append(attempts, n) }},
	)
	ch, err := w.Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := mwDrain(ch); mwTexts(got) != "ok" {
		t.Fatalf("events = %+v", got)
	}
	if m.calls.Load() != 3 || len(attempts) != 2 || attempts[1] != 2 {
		t.Fatalf("calls=%d attempts=%v", m.calls.Load(), attempts)
	}
}

func TestWithRetry_ExhaustsAndReturnsLastError(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{err: mwAPIErr(500)}}}
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 2}).Stream(context.Background(), llm.Request{})
	var api *aikit.APIError
	if !errors.As(err, &api) || m.calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, m.calls.Load())
	}
}

func TestWithRetry_ErrorEventBeforeContentRetriesAndDropsPriorEvents(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{
		{events: []aikit.StreamEvent{mwUsage(), mwFail(mwAPIErr(502))}},
		{events: []aikit.StreamEvent{mwText("fine"), mwFinish()}},
	}}
	ch, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	got := mwDrain(ch)
	if mwTexts(got) != "fine" || mwLastError(got) != nil || got[0].Type != aikit.StreamEventTextDelta {
		// got[0] must be the text of the second attempt, not the first attempt's usage.
		t.Fatalf("events = %+v", got)
	}
}

func TestWithRetry_NeverRetriesAfterContent(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{
		{events: []aikit.StreamEvent{mwText("partial"), mwFail(mwAPIErr(503))}},
		{events: []aikit.StreamEvent{mwText("second"), mwFinish()}},
	}}
	ch, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 3}).Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	got := mwDrain(ch)
	if mwTexts(got) != "partial" || mwLastError(got) == nil || m.calls.Load() != 1 {
		t.Fatalf("events=%+v calls=%d", got, m.calls.Load())
	}
}

func TestWithRetry_NeverRetriesProviderRejection(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422} {
		for name, retryable := range map[string]func(error) bool{
			"default":      nil,
			"always-retry": func(error) bool { return true },
		} {
			m := &mwScriptedModel{
				scripts: []mwScript{{err: mwAPIErr(status)}, {events: []aikit.StreamEvent{mwText("x")}}},
			}
			_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 3, Retryable: retryable}).
				Stream(context.Background(), llm.Request{})
			if err == nil || m.calls.Load() != 1 {
				t.Fatalf("status %d (%s): err=%v calls=%d", status, name, err, m.calls.Load())
			}
		}
	}
	// A rejection delivered as an error event is not retried either.
	m := &mwScriptedModel{
		scripts: []mwScript{
			{events: []aikit.StreamEvent{mwFail(mwAPIErr(400))}},
			{events: []aikit.StreamEvent{mwText("x")}},
		},
	}
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 3}).Stream(context.Background(), llm.Request{})
	if err == nil || m.calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, m.calls.Load())
	}
}

func TestWithRetry_UnclassifiedErrorsNotRetriedByDefault(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{err: errors.New("provider said no")}}}
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 3}).Stream(context.Background(), llm.Request{})
	if err == nil || m.calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, m.calls.Load())
	}
}

func TestWithRetry_DefaultClassifierRetriesTransportFailures(t *testing.T) {
	for name, cause := range map[string]error{
		"unexpected eof": io.ErrUnexpectedEOF,
		"stall":          &llm.StallTimeoutError{After: time.Second},
	} {
		m := &mwScriptedModel{scripts: []mwScript{{err: cause}, {events: []aikit.StreamEvent{mwText("ok")}}}}
		ch, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(context.Background(), llm.Request{})
		if err != nil || mwTexts(mwDrain(ch)) != "ok" {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

func TestWithRetry_CustomClassifier(t *testing.T) {
	sentinel := errors.New("custom transient")
	m := &mwScriptedModel{scripts: []mwScript{{err: sentinel}, {events: []aikit.StreamEvent{mwText("ok")}}}}
	w := llm.WithRetry(
		m,
		llm.RetryOptions{MaxRetries: 1, Retryable: func(err error) bool { return errors.Is(err, sentinel) }},
	)
	ch, err := w.Stream(context.Background(), llm.Request{})
	if err != nil || mwTexts(mwDrain(ch)) != "ok" {
		t.Fatalf("err=%v", err)
	}
}

func TestWithRetry_ContextCancelDuringBackoff(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{err: mwAPIErr(503)}}}
	ctx, cancel := context.WithCancel(context.Background())
	w := llm.WithRetry(m, llm.RetryOptions{
		MaxRetries: 5, InitialDelay: time.Minute,
		OnRetry: func(int, error) { go cancel() },
	})
	start := time.Now()
	_, err := w.Stream(ctx, llm.Request{})
	if !errors.Is(err, context.Canceled) || time.Since(start) > 5*time.Second || m.calls.Load() != 1 {
		t.Fatalf("err=%v elapsed=%v calls=%d", err, time.Since(start), m.calls.Load())
	}
}

func TestWithRetry_ContextCancelledBeforeCall(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{err: mwAPIErr(503)}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 2}).Stream(ctx, llm.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithRetry_CancelWhileAwaitingContent(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{hang: true}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 2}).Stream(ctx, llm.Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithRetry_BackoffAndRetryAfterCapped(t *testing.T) {
	retryAfter := aikit.NewAPIError("p", 429, nil)
	retryAfter.RetryAfter = time.Hour
	m := &mwScriptedModel{scripts: []mwScript{{err: retryAfter}, {events: []aikit.StreamEvent{mwText("ok")}}}}
	w := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1, InitialDelay: time.Hour, MaxDelay: 10 * time.Millisecond})
	start := time.Now()
	ch, err := w.Stream(context.Background(), llm.Request{})
	if err != nil || mwTexts(mwDrain(ch)) != "ok" || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
	// Exponential growth with a factor, capped.
	m2 := &mwScriptedModel{
		scripts: []mwScript{{err: mwAPIErr(503)}, {err: mwAPIErr(503)}, {events: []aikit.StreamEvent{mwText("ok")}}},
	}
	start = time.Now()
	ch, err = llm.WithRetry(m2, llm.RetryOptions{MaxRetries: 2, InitialDelay: 5 * time.Millisecond, BackoffFactor: 2}).
		Stream(context.Background(), llm.Request{})
	if err != nil || mwTexts(mwDrain(ch)) != "ok" || time.Since(start) < 14*time.Millisecond {
		t.Fatalf("err=%v elapsed=%v", err, time.Since(start))
	}
}

func TestWithRetry_ErrorEventWithoutErrorIsFailure(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{events: []aikit.StreamEvent{{Type: aikit.StreamEventError}}}}}
	_, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWithRetry_EmptyStreamPassesThrough(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{}}}
	ch, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(context.Background(), llm.Request{})
	if err != nil || len(mwDrain(ch)) != 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestWithRetry_ComposesWithStallTimeout(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{hang: true}, {events: []aikit.StreamEvent{mwText("ok"), mwFinish()}}}}
	w := llm.WithRetry(llm.WithStallTimeout(m, 30*time.Millisecond), llm.RetryOptions{MaxRetries: 1})
	ch, err := w.Stream(context.Background(), llm.Request{})
	if err != nil || mwTexts(mwDrain(ch)) != "ok" || m.calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, m.calls.Load())
	}
}

func TestWithRetry_CallerCancelAfterCommitReleasesStream(t *testing.T) {
	m := &mwScriptedModel{scripts: []mwScript{{events: []aikit.StreamEvent{mwText("a")}, hang: true}}}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := llm.WithRetry(m, llm.RetryOptions{MaxRetries: 1}).Stream(ctx, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	<-ch
	cancel()
	done := make(chan struct{})
	go func() { mwDrain(ch); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream not released")
	}
}

// A consumer slower than d must not be reported as a provider stall.
func TestWithStallTimeout_SlowConsumerIsNotAStall(t *testing.T) {
	const d = 40 * time.Millisecond
	var events []aikit.StreamEvent
	for i := 0; i < 20; i++ {
		events = append(events, mwText("x"))
	}
	events = append(events, mwFinish())
	// Provider emits steadily (no gap); the channel buffers (8) fill so the
	// wrapper blocks in forward() while the consumer sleeps.
	m := &mwScriptedModel{scripts: []mwScript{{events: events}}}
	ch, err := llm.WithStallTimeout(m, d).Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	var got []aikit.StreamEvent
	for ev := range ch {
		got = append(got, ev)
		time.Sleep(2 * d)
	}
	if mwLastError(got) != nil || mwTexts(got) != strings.Repeat("x", 20) {
		t.Fatalf("slow consumer caused failure: %+v", mwLastError(got))
	}
}

func TestWithStallTimeout_SilentProviderAfterSlowConsumerStillStalls(t *testing.T) {
	const d = 40 * time.Millisecond
	m := &mwScriptedModel{scripts: []mwScript{{events: []aikit.StreamEvent{mwText("a")}, hang: true}}}
	ch, err := llm.WithStallTimeout(m, d).Stream(context.Background(), llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	first := <-ch
	time.Sleep(3 * d) // consumer is slow, then the provider is silent
	got := append([]aikit.StreamEvent{first}, mwDrain(ch)...)
	var stall *llm.StallTimeoutError
	if !errors.As(mwLastError(got), &stall) {
		t.Fatalf("events = %+v", got)
	}
}
