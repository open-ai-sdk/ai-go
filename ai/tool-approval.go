package ai

import (
	"context"
	"encoding/json"
)

// ApprovalDecision is the outcome of the per-call approval check that runs
// before a tool executes.
type ApprovalDecision string

// ApprovalRequired suspends the tool call until a responder approves it.
const ApprovalRequired ApprovalDecision = "user-approval"

// ToolApprovalFunc decides whether a pending tool call needs human approval.
type ToolApprovalFunc func(toolName string, args json.RawMessage) ApprovalDecision

// ToolApprovalRequest describes a suspended tool call awaiting a decision.
// ApprovalID correlates the request with its response.
type ToolApprovalRequest struct {
	ApprovalID, ToolCallID, ToolName string
	Args                             json.RawMessage
}

// ToolApprovalResponse carries the decision for one ToolApprovalRequest.
// A call proceeds only when Approved is true; the default is deny.
type ToolApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}

// ToolApprovalResponder resolves an approval request, blocking until a decision
// is available or the context is cancelled.
type ToolApprovalResponder func(context.Context, ToolApprovalRequest) (ToolApprovalResponse, error)
