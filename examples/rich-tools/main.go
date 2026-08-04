// Package main demonstrates a rich local tool without requiring credentials.
package main

import (
	"context"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func main() {
	report, err := tool.New(
		"report",
		"Return text and structured data",
		func(_ context.Context, _ struct{}) (tool.ExecutionResult, error) {
			output, err := tool.Content(
				aikit.TextToolResultContent("report ready"),
				aikit.JSONToolResultContent(json.RawMessage(`{"rows":2}`)),
			)
			return tool.ExecutionResult{Output: output, Metadata: map[string]any{"source": "local"}}, err
		},
	)
	if err != nil {
		panic(err)
	}
	_, err = tool.NewSet(report)
	if err != nil {
		panic(err)
	}
}
