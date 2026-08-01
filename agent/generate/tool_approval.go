package generate

import (
	"context"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/agent"
)

type (
	ToolApprovalReplayGuard = agent.ApprovalReplayGuard
	ToolApprovalGrant       = agent.ApprovalGrant
)

func NewMemoryToolApprovalReplayGuard() *agent.MemoryApprovalReplayGuard {
	return agent.NewMemoryApprovalReplayGuard()
}

// ApprovalDecision is the outcome of the per-call approval check that runs
// before a tool executes.
type ApprovalDecision string

// ApprovalRequired gates the tool call until an in-process responder or a
// signed history response approves it.
const ApprovalRequired ApprovalDecision = "user-approval"

// ToolApprovalFunc decides whether a pending tool call needs human approval.
type ToolApprovalFunc func(toolName string, args json.RawMessage) ApprovalDecision

// ToolApprovalRequest describes a suspended tool call awaiting a decision.
// ApprovalID correlates the request with its response.
type ToolApprovalRequest struct {
	ApprovalID, ToolCallID, ToolName string
	Args                             json.RawMessage
	Signature                        string
}

// ToolApprovalResponse carries the decision for one ToolApprovalRequest.
// A call proceeds only when Approved is true; the default is deny.
type ToolApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}

// ToolApprovalResponder optionally resolves an approval request within the
// current invocation. When omitted, the run suspends and can be resumed by
// adding [ToolApprovalResponsePart] to the next request's message history.
type ToolApprovalResponder func(context.Context, ToolApprovalRequest) (ToolApprovalResponse, error)
