package agent

import "github.com/open-ai-sdk/ai-go/tool"

func testToolSet(definitions []ToolDefinition, executor tool.Executor) *ToolSet {
	set, err := tool.NewSetFromExecutor(definitions, executor)
	if err != nil {
		panic(err)
	}
	return set
}
