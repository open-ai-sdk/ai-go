package kie

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoll_DoneAfterN(t *testing.T) {
	calls := 0
	statusFn := func(ctx context.Context) (int, bool, error) {
		calls++
		return calls, calls == 3, nil
	}
	p := poller{Interval: 1 * time.Millisecond, MaxWait: 1 * time.Second}
	got, err := poll(context.Background(), p, statusFn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != 3 || calls != 3 {
		t.Errorf("got=%d calls=%d", got, calls)
	}
}

func TestPoll_CtxCancel(t *testing.T) {
	statusFn := func(ctx context.Context) (int, bool, error) {
		return 0, false, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	p := poller{Interval: 5 * time.Millisecond, MaxWait: 1 * time.Second}
	_, err := poll(ctx, p, statusFn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestPoll_Timeout(t *testing.T) {
	statusFn := func(ctx context.Context) (int, bool, error) {
		return 0, false, nil
	}
	p := poller{Interval: 5 * time.Millisecond, MaxWait: 20 * time.Millisecond}
	_, err := poll(context.Background(), p, statusFn)
	if !errors.Is(err, errPollTimeout) {
		t.Errorf("err = %v, want errPollTimeout", err)
	}
}

func TestPoll_StatusErrPropagates(t *testing.T) {
	want := errors.New("kaboom")
	statusFn := func(ctx context.Context) (int, bool, error) {
		return 0, false, want
	}
	p := poller{Interval: 1 * time.Millisecond, MaxWait: 1 * time.Second}
	_, err := poll(context.Background(), p, statusFn)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
