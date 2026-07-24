package ai

import (
	"context"
	"encoding/json"
)

type ApprovalDecision string

const ApprovalRequired ApprovalDecision = "user-approval"

type ToolApprovalFunc func(toolName string, args json.RawMessage) ApprovalDecision
type ToolApprovalRequest struct {
	ApprovalID, ToolCallID, ToolName string
	Args                             json.RawMessage
}
type ToolApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}
type ToolApprovalResponder func(context.Context, ToolApprovalRequest) (ToolApprovalResponse, error)
