package uistream

import (
	"context"
	"sync"

	"github.com/open-ai-sdk/ai-go/ai"
)

// ApprovalBroker correlates UI tool-approval responses with waiting tool calls.
type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]chan ai.ToolApprovalResponse
}

func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{pending: make(map[string]chan ai.ToolApprovalResponse)}
}
func (b *ApprovalBroker) Responder() ai.ToolApprovalResponder { return b.request }
func (b *ApprovalBroker) request(ctx context.Context, request ai.ToolApprovalRequest) (ai.ToolApprovalResponse, error) {
	ch := make(chan ai.ToolApprovalResponse, 1)
	b.mu.Lock()
	b.pending[request.ApprovalID] = ch
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, request.ApprovalID); b.mu.Unlock() }()
	select {
	case response := <-ch:
		return response, nil
	case <-ctx.Done():
		return ai.ToolApprovalResponse{}, ctx.Err()
	}
}

// Respond resumes the matching suspended tool call. It returns false when no call is waiting.
func (b *ApprovalBroker) Respond(response ai.ToolApprovalResponse) bool {
	b.mu.Lock()
	ch := b.pending[response.ApprovalID]
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- response
	return true
}
