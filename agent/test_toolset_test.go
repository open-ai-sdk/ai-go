package agent

import (
	"context"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/tool"
)

type testDispatcher interface {
	Execute(context.Context, string, string) (string, error)
}

type testDispatchTool struct {
	definition ToolDefinition
	dispatcher testDispatcher
}

func (t testDispatchTool) Describe() ToolDefinition { return t.definition }

func (t testDispatchTool) Invoke(
	ctx context.Context,
	input json.RawMessage,
) (json.RawMessage, error) {
	output, err := t.dispatcher.Execute(ctx, t.definition.Name, string(input))
	return json.RawMessage(output), err
}

func testToolSet(definitions []ToolDefinition, dispatcher testDispatcher) *ToolSet {
	if definitions == nil {
		definitions = testToolDefinitions()
	}
	tools := make([]tool.Invokable, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, testDispatchTool{
			definition: definition,
			dispatcher: dispatcher,
		})
	}
	set, err := tool.NewSet(tools...)
	if err != nil {
		panic(err)
	}
	return set
}

func testToolDefinitions() []ToolDefinition {
	names := []string{
		"Search", "boom", "deleteFile", "echo", "fetch", "first",
		"get_time", "lookup", "loop", "run", "search", "second",
		"skip", "slow", "work",
	}
	definitions := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, ToolDefinition{Name: name})
	}
	return definitions
}
