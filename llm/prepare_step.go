package llm

import "github.com/open-ai-sdk/ai-go/aikit"

// PrepareStepContext describes the tool-loop state before a model call.
type PrepareStepContext struct {
	StepNumber int
	Steps      []PrepareStepInfo
	// ToolsContext and RuntimeContext retain the request's shallow ownership.
	// Treat nested values as shared and synchronize any intentional mutation.
	ToolsContext   aikit.ToolsContext
	RuntimeContext aikit.RuntimeContext
}

// PrepareStepInfo describes one completed step.
type PrepareStepInfo struct {
	StepNumber       int
	MessageID        string
	HasToolCalls     bool
	ToolNames        []string
	Text             string
	Reasoning        string
	ToolCalls        []aikit.ToolCallInfo
	ToolResults      []aikit.ToolResult
	Usage            *aikit.Usage
	FinishReason     aikit.FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []aikit.Warning
}

// PrepareStepResult holds per-step request overrides.
type PrepareStepResult struct {
	Model           Model
	ToolChoice      *aikit.ToolChoice
	ActiveTools     []string
	Instructions    string
	ProviderOptions map[string]any
}

// PrepareStepFunc is called before each tool-loop step.
type PrepareStepFunc func(PrepareStepContext) *PrepareStepResult
