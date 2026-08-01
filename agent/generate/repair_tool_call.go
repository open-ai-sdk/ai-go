package generate

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

type (
	RepairToolCallInput = aikit.RepairToolCallInput
	RepairToolCallFunc  = aikit.RepairToolCallFunc
	NoSuchToolError     = tool.NoSuchToolError
	ToolInputError      = tool.InputError
)
