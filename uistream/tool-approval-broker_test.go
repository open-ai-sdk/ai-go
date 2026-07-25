package uistream

import (
	"context"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

func TestApprovalBrokerCorrelatesResponse(t *testing.T) {
	broker := NewApprovalBroker()
	result := make(chan ai.ToolApprovalResponse, 1)
	go func() {
		response, _ := broker.Responder()(context.Background(), ai.ToolApprovalRequest{ApprovalID: "a1"})
		result <- response
	}()
	deadline := time.Now().Add(time.Second)
	for !broker.Respond(ai.ToolApprovalResponse{ApprovalID: "a1", Approved: true}) {
		if time.Now().After(deadline) {
			t.Fatal("request was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if response := <-result; !response.Approved {
		t.Fatal("approval response was not delivered")
	}
}
