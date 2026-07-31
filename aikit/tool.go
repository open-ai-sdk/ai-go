package aikit

import (
	"context"
	"sync"
	"time"
)

// ToolDefinition describes a callable function tool available to a model.
type ToolDefinition struct {
	Name          string
	Description   string
	InputSchema   map[string]any
	ContextSchema map[string]any
	ToModelOutput func(result string) string
	Timeout       time.Duration
}

// ToolChoice controls which tool a model may call.
type ToolChoice struct {
	Type     string
	ToolName string
}

// ToolExecutor executes a named tool with JSON arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, name, argsJSON string) (string, error)
}

// ToolSet is a collection of tool definitions and an executor.
//
// Pass ToolSet by pointer. It contains a lazily initialized lookup index.
type ToolSet struct {
	Definitions []ToolDefinition
	Executor    ToolExecutor

	indexOnce sync.Once
	index     map[string]int
}

// Lookup returns the definition named name.
func (ts *ToolSet) Lookup(name string) (ToolDefinition, bool) {
	if ts == nil {
		return ToolDefinition{}, false
	}
	ts.indexOnce.Do(ts.buildIndex)
	if len(ts.index) != len(ts.Definitions) {
		return ts.scanForName(name)
	}
	i, ok := ts.index[name]
	if !ok {
		return ToolDefinition{}, false
	}
	return ts.Definitions[i], true
}

func (ts *ToolSet) buildIndex() {
	ts.index = make(map[string]int, len(ts.Definitions))
	for i, definition := range ts.Definitions {
		ts.index[definition.Name] = i
	}
}

func (ts *ToolSet) scanForName(name string) (ToolDefinition, bool) {
	for _, definition := range ts.Definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return ToolDefinition{}, false
}
