package engine

import "context"

type ApprovalRequest struct{ ApprovalID, ToolCallID, ToolName, Args string }
type ApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}
type ApprovalResponder interface {
	RequestApproval(context.Context, ApprovalRequest) (ApprovalResponse, error)
}
