package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

const defaultMaxTurns = 1

var (
	errNilModel             = errors.New("model is nil")
	errInvalidMaxTurns      = errors.New("max turns must be at least 1")
	errInvalidConcurrency   = errors.New("tool concurrency must be at least 1")
	errInvalidApprovalKey   = errors.New("approval key must contain at least 32 bytes")
	errMissingApprovalKey   = errors.New("approval key is required when approval may suspend")
	errNilApprovalPolicy    = errors.New("approval policy is nil")
	errNilHook              = errors.New("hook is nil")
	errUnknownActiveTool    = errors.New("active tool is not registered")
	errDuplicateActiveTool  = errors.New("active tool is listed more than once")
	errInvalidToolChoice    = errors.New("tool choice type is invalid")
	errImpossibleToolChoice = errors.New("tool choice cannot be satisfied by the active tools")
)

// BuildError reports an invalid long-lived Agent default. Field identifies the
// Builder method whose value failed validation. Unwrap exposes the underlying
// validation or registry error.
type BuildError struct {
	Field string
	Err   error
}

func (e *BuildError) Error() string {
	if e == nil {
		return "agent: build failed"
	}
	if e.Field == "" {
		return fmt.Sprintf("agent: build: %v", e.Err)
	}
	return fmt.Sprintf("agent: build %s: %v", e.Field, e.Err)
}

func (e *BuildError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Builder configures the long-lived defaults of an Agent. It is a value
// builder: fluent calls return an independent top-level value, and Build takes
// a defensive snapshot of all mutable configuration.
type Builder struct {
	config config
}

// New starts an Agent builder bound to model.
func New(model llm.Model) Builder {
	return Builder{config: config{
		model:            model,
		maxTurns:         defaultMaxTurns,
		maxParallelTools: 1,
	}}
}

// ID sets the stable Agent identifier.
func (b Builder) ID(id string) Builder {
	b.config.id = id
	return b
}

// Instructions sets the default system instructions.
func (b Builder) Instructions(instructions string) Builder {
	b.config.instructions = instructions
	return b
}

// Tools sets the default immutable tool registry.
func (b Builder) Tools(set *tool.Set) Builder {
	b.config.tools = set
	return b
}

// ToolChoice sets the default model tool-selection policy.
func (b Builder) ToolChoice(choice aikit.ToolChoice) Builder {
	b.config.toolChoice = cloneToolChoice(&choice)
	return b
}

// ActiveTools sets the default allowlist. Calling ActiveTools with no names is
// an explicit empty allowlist; not calling it means all registered tools.
func (b Builder) ActiveTools(names ...string) Builder {
	b.config.activeTools = append([]string{}, names...)
	return b
}

// MaxTurns sets the total number of model calls available to one run.
func (b Builder) MaxTurns(maxTurns int) Builder {
	b.config.maxTurns = maxTurns
	return b
}

// StopWhen sets an optional early-stop condition. MaxTurns remains the hard
// upper bound on model calls.
func (b Builder) StopWhen(condition aikit.StopCondition) Builder {
	b.config.stopWhen = condition
	return b
}

// Output sets the default structured-output schema.
func (b Builder) Output(output llm.OutputSchema) Builder {
	b.config.output = cloneOutputSchema(&output)
	return b
}

// Settings replaces the common model call settings.
func (b Builder) Settings(settings llm.CallSettings) Builder {
	b.config.settings = cloneCallSettings(settings)
	return b
}

// Temperature sets the default sampling temperature.
func (b Builder) Temperature(temperature float32) Builder {
	b.config.settings.Temperature = clonePointer(&temperature)
	return b
}

// MaxTokens sets the default maximum output-token count.
func (b Builder) MaxTokens(maxTokens int) Builder {
	b.config.settings.MaxTokens = maxTokens
	return b
}

// TopP sets the default nucleus-sampling probability mass.
func (b Builder) TopP(topP float32) Builder {
	b.config.settings.TopP = clonePointer(&topP)
	return b
}

// TopK sets the default next-token candidate limit.
func (b Builder) TopK(topK int) Builder {
	b.config.settings.TopK = clonePointer(&topK)
	return b
}

// Seed sets the default deterministic sampling seed.
func (b Builder) Seed(seed int) Builder {
	b.config.settings.Seed = clonePointer(&seed)
	return b
}

// StopSequences replaces the default stop sequences.
func (b Builder) StopSequences(sequences ...string) Builder {
	b.config.settings.StopSequences = append([]string(nil), sequences...)
	return b
}

// ProviderOptions replaces all provider-specific default options.
func (b Builder) ProviderOptions(options map[string]any) Builder {
	b.config.providerOptions = snapshotJSONMap(options)
	return b
}

// ProviderOptionsJSON sets JSON-decoded defaults for one provider.
func (b Builder) ProviderOptionsJSON(provider string, options map[string]any) Builder {
	b.config.providerOptions = snapshotJSONMap(b.config.providerOptions)
	if b.config.providerOptions == nil {
		b.config.providerOptions = make(map[string]any)
	}
	b.config.providerOptions[provider] = snapshotJSONMap(options)
	return b
}

// With attaches typed provider-specific defaults. A later option for the same
// provider replaces the earlier value.
func (b Builder) With(option llm.ProviderOption) Builder {
	if llm.IsNilProviderOption(option) {
		return b
	}
	b.config.providerOptions = snapshotJSONMap(b.config.providerOptions)
	if b.config.providerOptions == nil {
		b.config.providerOptions = make(map[string]any)
	}
	b.config.providerOptions[option.ProviderName()] = option
	return b
}

// PrepareStep sets the callback that can adjust each model turn.
func (b Builder) PrepareStep(prepare llm.PrepareStepFunc) Builder {
	b.config.prepareStep = prepare
	return b
}

// RepairToolCall sets the default invalid tool-call repair callback.
func (b Builder) RepairToolCall(repair aikit.RepairToolCallFunc) Builder {
	b.config.repairToolCall = repair
	return b
}

// ToolsContext replaces the default per-tool context map.
func (b Builder) ToolsContext(context aikit.ToolsContext) Builder {
	b.config.toolsContext = cloneMap(context)
	return b
}

// RuntimeContext replaces the default run-wide tool context map.
func (b Builder) RuntimeContext(context aikit.RuntimeContext) Builder {
	b.config.runtimeContext = cloneMap(context)
	return b
}

// ToolApproval replaces the default per-tool approval policies.
func (b Builder) ToolApproval(policies map[string]ApprovalPolicy) Builder {
	b.config.toolApproval = cloneMap(policies)
	return b
}

// ApprovalKey sets the HMAC key used for stateless approval suspension and
// resumption.
func (b Builder) ApprovalKey(key []byte) Builder {
	b.config.approvalKey = append([]byte(nil), key...)
	return b
}

// ApprovalReplayGuard sets the capability replay guard.
func (b Builder) ApprovalReplayGuard(guard ApprovalReplayGuard) Builder {
	b.config.approvalReplayGuard = guard
	return b
}

// Approver sets an optional in-process approval responder.
func (b Builder) Approver(approver ApprovalResponder) Builder {
	b.config.approver = approver
	return b
}

// ToolConcurrency sets the maximum number of tool calls executed at once.
// Values above one enable parallel execution; one preserves serial order.
func (b Builder) ToolConcurrency(max int) Builder {
	b.config.maxParallelTools = max
	b.config.parallelToolExecution = max > 1
	return b
}

// Logger sets the structured logger used by runs.
func (b Builder) Logger(logger *slog.Logger) Builder {
	b.config.logger = logger
	return b
}

// Tracer sets the provider-neutral run tracer.
func (b Builder) Tracer(tracer Tracer) Builder {
	b.config.tracer = tracer
	return b
}

// TraceContent controls whether tracing may include prompt, completion, and
// tool argument content.
func (b Builder) TraceContent(enabled bool) Builder {
	b.config.traceContent = enabled
	return b
}

// Hook appends a lifecycle hook. Hooks execute in registration order.
func (b Builder) Hook(hook Hook) Builder {
	b.config.hooks = append(append([]Hook(nil), b.config.hooks...), hook)
	return b
}

// Build validates and snapshots the configuration into an immutable Agent.
func (b Builder) Build() (*Agent, error) {
	if isNilInterface(b.config.model) {
		return nil, buildError("Model", errNilModel)
	}
	if b.config.maxTurns < 1 {
		return nil, buildError("MaxTurns", errInvalidMaxTurns)
	}
	if b.config.maxParallelTools < 1 {
		return nil, buildError("ToolConcurrency", errInvalidConcurrency)
	}
	if b.config.tools != nil {
		if err := b.config.tools.Validate(); err != nil {
			return nil, buildError("Tools", err)
		}
	}
	if err := validateActiveTools(b.config); err != nil {
		return nil, err
	}
	if err := validateOutputConfiguration(b.config.output); err != nil {
		return nil, buildError("Output", err)
	}
	for name, policy := range b.config.toolApproval {
		if policy == nil {
			return nil, buildError("ToolApproval["+name+"]", errNilApprovalPolicy)
		}
		if b.config.tools == nil {
			return nil, buildError("ToolApproval["+name+"]", errUnknownActiveTool)
		}
		if _, exists := b.config.tools.Lookup(name); !exists {
			return nil, buildError("ToolApproval["+name+"]", errUnknownActiveTool)
		}
	}
	if len(b.config.approvalKey) > 0 && len(b.config.approvalKey) < minApprovalKeyBytes {
		return nil, buildError("ApprovalKey", errInvalidApprovalKey)
	}
	if len(b.config.toolApproval) > 0 &&
		isNilInterface(b.config.approver) &&
		len(b.config.approvalKey) < minApprovalKeyBytes {
		return nil, buildError("ApprovalKey", errMissingApprovalKey)
	}
	for i, hook := range b.config.hooks {
		if isNilInterface(hook) {
			return nil, buildError(fmt.Sprintf("Hook[%d]", i), errNilHook)
		}
	}

	configured := cloneConfig(b.config)
	if configured.id == "" {
		configured.id = configured.model.ModelID()
	}
	return &Agent{config: configured}, nil
}

func validateActiveTools(config config) error {
	seen := make(map[string]struct{}, len(config.activeTools))
	for _, name := range config.activeTools {
		if _, duplicate := seen[name]; duplicate {
			return buildError("ActiveTools", fmt.Errorf("%w: %q", errDuplicateActiveTool, name))
		}
		seen[name] = struct{}{}
	}
	if config.activeTools != nil {
		for _, name := range config.activeTools {
			if config.tools == nil {
				return buildError("ActiveTools", fmt.Errorf("%w: %q", errUnknownActiveTool, name))
			}
			if _, exists := config.tools.Lookup(name); !exists {
				return buildError("ActiveTools", fmt.Errorf("%w: %q", errUnknownActiveTool, name))
			}
		}
	}
	if config.toolChoice == nil {
		return nil
	}
	choice := *config.toolChoice
	switch choice.Type {
	case "none", "auto", "":
		return nil
	case "required":
		if availableToolCount(config) == 0 {
			return buildError("ToolChoice", errImpossibleToolChoice)
		}
	case "tool":
		if choice.ToolName == "" || !isToolActive(config, choice.ToolName) {
			return buildError("ToolChoice", fmt.Errorf("%w: %q", errImpossibleToolChoice, choice.ToolName))
		}
	default:
		return buildError("ToolChoice", fmt.Errorf("%w: %q", errInvalidToolChoice, choice.Type))
	}
	return nil
}

func availableToolCount(config config) int {
	if config.activeTools != nil {
		return len(config.activeTools)
	}
	if config.tools == nil {
		return 0
	}
	return config.tools.Len()
}

func isToolActive(config config, name string) bool {
	if config.tools == nil {
		return false
	}
	if _, exists := config.tools.Lookup(name); !exists {
		return false
	}
	if config.activeTools == nil {
		return true
	}
	for _, active := range config.activeTools {
		if active == name {
			return true
		}
	}
	return false
}

func buildError(field string, err error) *BuildError {
	return &BuildError{Field: field, Err: err}
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
