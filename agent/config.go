package agent

import (
	"log/slog"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// ApprovalPolicy decides whether a tool call must be approved before it runs.
// The arguments are the tool name and its raw JSON input.
type ApprovalPolicy = func(toolName, args string) bool

// Agent is a reusable, immutable agent configuration. Create one with [New]
// and [Builder.Build]. Per-invocation input and overrides belong to Runner.
type Agent struct {
	config config
}

// config contains only long-lived defaults. Builder.Build snapshots every
// mutable value before placing it here; Runner snapshots it again so a run can
// safely add overrides without mutating the Agent.
type config struct {
	model        llm.Model
	id           string
	instructions string

	tools       *tool.Set
	toolChoice  *aikit.ToolChoice
	activeTools []string
	maxTurns    int
	stopWhen    aikit.StopCondition
	output      *llm.OutputSchema
	outputMode  OutputMode

	settings        llm.CallSettings
	providerOptions map[string]any
	prepareStep     llm.PrepareStepFunc
	repairToolCall  aikit.RepairToolCallFunc
	toolsContext    aikit.ToolsContext
	runtimeContext  aikit.RuntimeContext

	toolApproval        map[string]ApprovalPolicy
	approvalKey         []byte
	approvalReplayGuard ApprovalReplayGuard
	approver            ApprovalResponder

	parallelToolExecution bool
	maxParallelTools      int
	logger                *slog.Logger
	tracer                Tracer
	traceContent          bool
	hooks                 []Hook
}

// ID returns the stable identifier configured for the Agent. When Builder.ID
// is not called, Build uses the model ID.
func (a *Agent) ID() string {
	if a == nil {
		return ""
	}
	return a.config.id
}

// Instructions returns the Agent's default system instructions.
func (a *Agent) Instructions() string {
	if a == nil {
		return ""
	}
	return a.config.instructions
}

// MaxTurns returns the total model-call budget for each Runner.
func (a *Agent) MaxTurns() int {
	if a == nil {
		return 0
	}
	return a.config.maxTurns
}

func cloneConfig(source config) config {
	cloned := source
	cloned.tools = cloneToolSet(source.tools)
	cloned.toolChoice = cloneToolChoice(source.toolChoice)
	cloned.activeTools = append([]string(nil), source.activeTools...)
	if source.activeTools != nil && cloned.activeTools == nil {
		cloned.activeTools = []string{}
	}
	cloned.output = cloneOutputSchema(source.output)
	cloned.settings = cloneCallSettings(source.settings)
	cloned.providerOptions = snapshotJSONMap(source.providerOptions)
	cloned.toolsContext = cloneMap(source.toolsContext)
	cloned.runtimeContext = cloneMap(source.runtimeContext)
	cloned.toolApproval = cloneMap(source.toolApproval)
	cloned.approvalKey = append([]byte(nil), source.approvalKey...)
	cloned.hooks = append([]Hook(nil), source.hooks...)
	return cloned
}

func cloneToolSet(source *tool.Set) *tool.Set {
	if source == nil {
		return nil
	}
	return source.Clone()
}

func cloneToolChoice(source *aikit.ToolChoice) *aikit.ToolChoice {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneOutputSchema(source *llm.OutputSchema) *llm.OutputSchema {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.Schema = snapshotJSONMap(source.Schema)
	return &cloned
}

func cloneCallSettings(source llm.CallSettings) llm.CallSettings {
	cloned := source
	cloned.Temperature = clonePointer(source.Temperature)
	cloned.TopP = clonePointer(source.TopP)
	cloned.TopK = clonePointer(source.TopK)
	cloned.Seed = clonePointer(source.Seed)
	cloned.StopSequences = append([]string(nil), source.StopSequences...)
	return cloned
}

func clonePointer[T any](source *T) *T {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneMap[M ~map[K]V, K comparable, V any](source M) M {
	if source == nil {
		return nil
	}
	cloned := make(M, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
