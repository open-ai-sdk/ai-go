package ai

import (
	"github.com/open-ai-sdk/ai-go/aikit"
)

// Tool error types are aliases of the shared aikit definitions (see
// aikit/tool-errors.go and ai/errors.go).
type (
	RepairToolCallInput       = aikit.RepairToolCallInput
	RepairToolCallFunc        = aikit.RepairToolCallFunc
	NoSuchToolError           = aikit.NoSuchToolError
	InvalidToolArgumentsError = aikit.InvalidToolArgumentsError
)
