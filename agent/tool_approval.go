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

// ApprovalResponder optionally resolves an approval request within the current
// invocation. When it is nil, the run suspends after emitting the request; the
// caller resumes statelessly by adding a tool_approval_response content part to
// the next request's message history.
type ApprovalResponder interface {
	RequestApproval(context.Context, ApprovalRequest) (ApprovalResponse, error)
}
