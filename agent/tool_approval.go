package agent

import (
	"context"
	"errors"
)

var errApprovalPending = errors.New("agent: tool approval pending")

// ApprovalRequest describes a suspended tool call awaiting a decision.
// ApprovalID correlates the request with its response.
type ApprovalRequest struct{ ApprovalID, ToolCallID, ToolName, Args string }

// ApprovalResponse carries the decision for one ApprovalRequest.
// A call proceeds only when Approved is true; the default is deny.
type ApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}

// ApprovalResponder resolves an approval request, blocking until a decision is
// available or the context is cancelled.
type ApprovalResponder interface {
	RequestApproval(context.Context, ApprovalRequest) (ApprovalResponse, error)
}
