package engine

import (
	"context"
	"sync"
	"time"
)

// ToolExecutor executes a named tool with JSON arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, name, argsJSON string) (string, error)
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name          string
	Description   string
	InputSchema   map[string]any
	ContextSchema map[string]any
	ToModelOutput func(result string) string
	// Timeout bounds a single Execute call for this tool. Zero means no bound
	// — see ai.ToolDefinition.Timeout for the full rationale.
	Timeout time.Duration
}

// ToolSet is a collection of tool definitions and an executor.
type ToolSet struct {
	Definitions []ToolDefinition
	Executor    ToolExecutor

	// indexOnce/index back Lookup with an O(1) name→definition map. See
	// ai.ToolSet.Lookup (the public mirror of this type) for the rationale.
	indexOnce sync.Once
	index     map[string]int
}

// Lookup returns the ToolDefinition named name in O(1) after the first call,
// which lazily builds a name→index map from Definitions. Falls back to a
// linear scan if Definitions was mutated after the index was built (detected
// via a length mismatch), so a stale index never produces a false miss.
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
	for i, d := range ts.Definitions {
		ts.index[d.Name] = i
	}
}

func (ts *ToolSet) scanForName(name string) (ToolDefinition, bool) {
	for _, d := range ts.Definitions {
		if d.Name == name {
			return d, true
		}
	}
	return ToolDefinition{}, false
}
