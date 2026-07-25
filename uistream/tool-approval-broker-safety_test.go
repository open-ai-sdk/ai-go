package uistream

import (
	"context"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// waitRegistered blocks until the broker accepts a response for id (i.e. a
// waiter has registered), or fails the test after a short deadline.
func waitRegistered(t *testing.T, broker *ApprovalBroker, resp ai.ToolApprovalResponse) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !broker.Respond(resp) {
		if time.Now().After(deadline) {
			t.Fatal("request was not registered")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestApprovalBroker_RespondTwice verifies a second Respond for the same ID
// returns false and neither call blocks.
func TestApprovalBroker_RespondTwice(t *testing.T) {
	broker := NewApprovalBroker()
	result := make(chan ai.ToolApprovalResponse, 1)
	go func() {
		response, _ := broker.Responder()(context.Background(), ai.ToolApprovalRequest{ApprovalID: "a1"})
		result <- response
	}()

	waitRegistered(t, broker, ai.ToolApprovalResponse{ApprovalID: "a1", Approved: true})

	// The waiter has been resolved and the entry claimed; a second response for
	// the same ID must report that no call is waiting — and must not block.
	done := make(chan bool, 1)
	go func() { done <- broker.Respond(ai.ToolApprovalResponse{ApprovalID: "a1", Approved: true}) }()
	select {
	case ok := <-done:
		if ok {
			t.Error("second Respond for a resolved ID should return false")
		}
	case <-time.After(time.Second):
		t.Fatal("second Respond blocked")
	}

	if response := <-result; !response.Approved {
		t.Fatal("first approval response was not delivered")
	}
}

// TestApprovalBroker_RespondAfterCancel verifies that once the waiter's context
// is cancelled, a later Respond returns false and does not block. Receiving the
// waiter's error guarantees its deferred cleanup of the pending entry has run.
func TestApprovalBroker_RespondAfterCancel(t *testing.T) {
	broker := NewApprovalBroker()
	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := broker.Responder()(ctx, ai.ToolApprovalRequest{ApprovalID: "b1"})
		result <- err
	}()

	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Error("expected context cancellation error from the waiter")
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not return after cancellation")
	}

	// After cancellation the pending entry is gone; Respond returns false, no block.
	done := make(chan bool, 1)
	go func() { done <- broker.Respond(ai.ToolApprovalResponse{ApprovalID: "b1", Approved: true}) }()
	select {
	case ok := <-done:
		if ok {
			t.Error("Respond after cancellation should return false")
		}
	case <-time.After(time.Second):
		t.Fatal("Respond after cancellation blocked")
	}
}
