package ai

import (
	"context"
	"sync"
	"time"

	"github.com/open-ai-sdk/ai-go/aitypes"
)

// ToolDefinition describes a callable function tool available to the model.
type ToolDefinition struct {
	// Name is the function name the model uses to invoke the tool.
	Name string
	// Description explains what the tool does; the model uses this for selection.
	Description string
	// InputSchema is a JSON Schema object describing the tool's input parameters.
	// Use the schema package to build this map, or construct it manually.
	InputSchema map[string]any
	// ContextSchema describes the optional per-tool context value supplied in ToolsContext.
	ContextSchema map[string]any
	// ToModelOutput optionally transforms the tool execution result before it
	// enters the conversation history. The original output is still reported in
	// ToolResult events. If nil, the raw output is used as-is.
	ToModelOutput func(result string) string
	// Timeout bounds a single Execute call for this tool. Zero (the default)
	// means no bound: agent tools legitimately run for minutes, and node has no
	// equivalent default, so ai-go does not invent one. Opt in per tool when a
	// runaway call should be cut short instead of the whole run.
	Timeout time.Duration
}

// ToolChoice controls which (if any) tool the model must call.
// Use the ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired constants or
// ToolChoiceSpecific to require a named tool.
type ToolChoice struct {
	// Type is one of "auto", "none", "required", or "tool".
	Type string
	// ToolName is set when Type == "tool" to name the required tool.
	ToolName string
}

var (
	// ToolChoiceAuto lets the model decide whether and which tool to call (default).
	ToolChoiceAuto = ToolChoice{Type: "auto"}
	// ToolChoiceNone prevents the model from calling any tool.
	ToolChoiceNone = ToolChoice{Type: "none"}
	// ToolChoiceRequired forces the model to call at least one tool.
	ToolChoiceRequired = ToolChoice{Type: "required"}
)

// ToolChoiceSpecific returns a ToolChoice that forces the model to call toolName.
func ToolChoiceSpecific(toolName string) ToolChoice {
	return ToolChoice{Type: "tool", ToolName: toolName}
}

// Tool-result content kinds and the ToolResult/ToolResultContent types are
// aliases of the shared aitypes package (see ai/types.go).
const (
	ToolResultContentTypeText = aitypes.ToolResultContentTypeText
	ToolResultContentTypeFile = aitypes.ToolResultContentTypeFile
)

type (
	ToolResultContent = aitypes.ToolResultContent
	ToolResult        = aitypes.ToolResult
)

// ToolExecutor executes a named tool with JSON arguments and returns a result string.
type ToolExecutor interface {
	Execute(ctx context.Context, name, argsJSON string) (string, error)
}

// ToolResultStream allows tools to stream partial output to the UI in real-time.
type ToolResultStream interface {
	// Write sends a partial result to the UI (e.g., stdout from a bash command).
	Write(partial string)
}

// StreamingToolExecutor extends ToolExecutor with streaming support.
// Tools that implement this interface receive a stream for real-time output.
type StreamingToolExecutor interface {
	ToolExecutor
	// ExecuteStreaming executes a tool with a stream for partial results.
	// Falls back to Execute if not implemented.
	ExecuteStreaming(ctx context.Context, name, argsJSON string, stream ToolResultStream) (string, error)
}

// ToolSet is a named collection of tool definitions and an executor.
//
// Pass it by pointer, not by value: it carries a lazily-built lookup index
// guarded by a sync.Once, so copying a ToolSet after (or across) a Lookup would
// copy that lock and share the index map. Everything in this module takes
// *ToolSet.
type ToolSet struct {
	Definitions []ToolDefinition
	Executor    ToolExecutor

	// indexOnce/index back Lookup with an O(1) name→definition map, built
	// lazily from Definitions on first Lookup call so a ToolSet that is never
	// looked up (e.g. one only ranged over for Definitions) pays nothing.
	indexOnce sync.Once
	index     map[string]int // tool name -> index into Definitions
}

// Lookup returns the ToolDefinition named name, building an internal
// name→index map from Definitions on first call so repeated lookups are O(1)
// instead of rescanning Definitions per call.
//
// Definitions must be fully populated before the first Lookup call; ToolSet is
// not a mutable registry. If Definitions is nonetheless mutated afterward,
// Lookup detects the length mismatch against the built index and falls back to
// a linear scan rather than trusting a stale index or returning a false miss.
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
