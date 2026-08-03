// ai-go: file-length-justification: keeps synchronized stream aggregation and its dependent result views in one state owner.
package generate

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// ErrStreamConsumed is delivered to a view registered after the source stream
// has already been fully consumed. Channel views surface it as a trailing
// StepEventError; Events() yields it. A late view is never a silently-closed
// empty channel.
var ErrStreamConsumed = errors.New("ai: stream already fully consumed")

// StreamResult wraps a streaming response with convenient accessors. It fans the
// single source channel out to any number of views — Stream, TextStream, Events,
// and Consume — created on demand.
//
// Views may be created in any order, before or after consumption starts. Each
// registers its own branch of the fan-out; a branch created after the source has
// advanced receives events from that point on, and one created after the source
// is exhausted receives ErrStreamConsumed rather than an empty channel.
//
// Backpressure: the source advances at the speed of the slowest *live* branch
// (each branch has a bounded buffer). A branch whose consumer has explicitly
// stopped — an Events() range that breaks — is unregistered and dropped, so it
// cannot wedge the source or the other views. Channel views (Stream/TextStream)
// have no break signal; their consumers are expected to drain to completion.
type StreamResult struct {
	src   <-chan StepEvent
	tools *ToolSet

	mu       sync.Mutex
	branches []*streamBranch
	started  bool
	srcDone  bool

	// done is closed when the fan-out goroutine has finished.
	done chan struct{}
}

// streamBranch is one registered view of the fan-out. done is closed when the
// branch unregisters so the producer's select drops it instead of blocking.
type streamBranch struct {
	ch   chan StepEvent
	done chan struct{}
}

// NewStreamResult wraps an engine step-event channel in a StreamResult.
func NewStreamResult(ch <-chan StepEvent) *StreamResult {
	return NewStreamResultWithTools(ch, nil)
}

// NewStreamResultWithTools wraps an engine step-event channel and preserves the
// tool definitions required to build response.messages in Consume().
func NewStreamResultWithTools(ch <-chan StepEvent, tools *ToolSet) *StreamResult {
	return &StreamResult{src: ch, tools: tools, done: make(chan struct{})}
}

// register creates a new branch of the fan-out. When the source is already
// exhausted it returns a branch pre-loaded with an ErrStreamConsumed event and a
// non-nil error, so every view surfaces the condition rather than a silent close.
func (sr *StreamResult) register() (*streamBranch, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.srcDone {
		b := &streamBranch{ch: make(chan StepEvent, 1), done: make(chan struct{})}
		b.ch <- StepEvent{Type: StepEventError, Error: ErrStreamConsumed}
		close(b.ch)
		return b, ErrStreamConsumed
	}
	b := &streamBranch{ch: make(chan StepEvent, 64), done: make(chan struct{})}
	sr.branches = append(sr.branches, b)
	return b, nil
}

// unregister removes b from sr's fan-out and signals the producer to drop it. It
// is safe to call more than once. The branch channel is intentionally not closed
// here: its consumer has already stopped reading, and finish() only closes
// branches still registered.
func (b *streamBranch) unregister(sr *StreamResult) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	for i, cur := range sr.branches {
		if cur == b {
			sr.branches = append(sr.branches[:i], sr.branches[i+1:]...)
			close(b.done)
			return
		}
	}
}

// ensureStarted launches the fan-out goroutine exactly once.
func (sr *StreamResult) ensureStarted() {
	sr.mu.Lock()
	if sr.started {
		sr.mu.Unlock()
		return
	}
	sr.started = true
	sr.mu.Unlock()

	go func() {
		defer sr.finish()
		defer safego.Recover(nil, func(err error) { sr.broadcast(StepEvent{Type: StepEventError, Error: err}) })
		for ev := range sr.src {
			sr.broadcast(ev)
		}
	}()
}

// broadcast delivers ev to every live branch. A slow but live branch applies
// backpressure (blocking send on its bounded buffer); an unregistered branch is
// dropped via its done channel.
func (sr *StreamResult) broadcast(ev StepEvent) {
	sr.mu.Lock()
	branches := append([]*streamBranch(nil), sr.branches...)
	sr.mu.Unlock()
	for _, b := range branches {
		select {
		case b.ch <- snapshotStepEvent(ev):
		case <-b.done:
		}
	}
}

// finish marks the source exhausted and closes every still-registered branch.
func (sr *StreamResult) finish() {
	sr.mu.Lock()
	sr.srcDone = true
	branches := sr.branches
	sr.branches = nil
	sr.mu.Unlock()
	for _, b := range branches {
		close(b.ch)
	}
	close(sr.done)
}

// TextStream returns a channel yielding text deltas only. Closed when the stream
// completes. A stream already consumed yields a closed channel (a text view has
// no error channel; use Stream or Events to observe ErrStreamConsumed).
func (sr *StreamResult) TextStream() <-chan string {
	b, err := sr.register()
	out := make(chan string, 64)
	if err != nil {
		close(out)
		return out
	}
	sr.ensureStarted()
	go func() {
		defer close(out)
		defer safego.Recover(nil, nil)
		defer b.unregister(sr)
		for ev := range b.ch {
			if ev.Type == StepEventTextDelta {
				out <- ev.TextDelta
			}
		}
	}()
	return out
}

// Stream returns the raw engine StepEvent channel, closed when the stream
// completes. This is an escape hatch for callers such as aisdk.Adapter that
// need full event visibility.
//
// Unlike TextStream/Events, this channel has no break signal: abandoning it
// without draining leaves the branch registered, so once its buffer fills the
// producer blocks — stalling every other live view of the same result. Drain it
// to completion, or cancel the StreamText context to tear the whole run down.
func (sr *StreamResult) Stream() <-chan StepEvent {
	b, err := sr.register()
	if err == nil {
		sr.ensureStarted()
	}
	return b.ch
}

// DrainUnused exists for backward compatibility and to satisfy the
// aisdk.StreamEventer interface. With the on-demand fan-out it is a no-op:
// unrequested views are never created, so there is nothing to drain. Safe to
// call any number of times, in any order relative to the view methods.
func (sr *StreamResult) DrainUnused() {}

// ConsumeStream drives the stream to completion without exposing any output,
// for fire-and-forget usage where side effects happen via callbacks. It
// registers one discard branch and drains it in the background.
func (sr *StreamResult) ConsumeStream() {
	b, err := sr.register()
	if err != nil {
		return
	}
	sr.ensureStarted()
	go func() {
		defer safego.Recover(nil, nil)
		defer b.unregister(sr)
		for range b.ch {
		}
	}()
}

// Consume blocks until the stream completes and returns the aggregated result.
// It reads its own branch so it does not interfere with other views.
func (sr *StreamResult) Consume() (*GenerateTextResult, error) {
	b, err := sr.register()
	if err != nil {
		return nil, err
	}
	sr.ensureStarted()
	defer b.unregister(sr)

	state := consumeState{result: &GenerateTextResult{}, tools: sr.tools}
	for ev := range b.ch {
		if terminal, terminalErr := state.consume(ev); terminal {
			state.finishResponse()
			return state.result, terminalErr
		}
	}
	state.finishResponse()
	return state.result, nil
}

type consumeState struct {
	result             *GenerateTextResult
	currentStep        *StepOutput
	currentUsage       Usage
	preludeToolResults []ToolResult
	tools              *ToolSet
}

func (state *consumeState) finishResponse() {
	state.result.Response = Response{Messages: responseMessagesWithPrelude(
		state.preludeToolResults, state.result.Steps, state.tools,
	)}
}

func (state *consumeState) consume(ev StepEvent) (bool, error) {
	switch ev.Type {
	case StepEventStepStart:
		state.currentStep = &StepOutput{}
		state.currentUsage = Usage{}
	case StepEventTextDelta:
		state.consumeText(ev)
	case StepEventReasoningDelta:
		state.consumeReasoning(ev)
	case StepEventToolCallStart:
		handleToolCallStart(ev, state.currentStep)
	case StepEventToolCallDelta:
		handleToolCallDelta(ev, state.currentStep)
	case StepEventToolCallReady:
		handleToolCallReady(ev, state.currentStep)
	case StepEventToolResult:
		state.consumeToolResult(ev)
	case StepEventUsage:
		handleUsage(ev, state.result, state.currentStep, &state.currentUsage)
	case StepEventSource:
		handleSource(ev, state.result, state.currentStep)
	case StepEventFileDelta:
		handleFileDelta(ev, state.result, state.currentStep)
	case StepEventStepEnd:
		handleStepEnd(ev, state.result, state.currentStep, state.tools)
		state.currentStep = nil
	case StepEventStructuredOutput:
		state.result.StructuredOutput = ev.StructuredOutput
	case StepEventToolApprovalRequest:
		state.consumeApprovalRequest(ev)
	case StepEventError:
		return true, ev.Error
	}
	return false, nil
}

func (state *consumeState) consumeText(ev StepEvent) {
	state.result.Text += ev.TextDelta
	if state.currentStep != nil {
		state.currentStep.Text += ev.TextDelta
		appendStepText(state.currentStep, ev.TextDelta, ev.ThoughtSignature)
	}
}

func (state *consumeState) consumeReasoning(ev StepEvent) {
	state.result.Reasoning += ev.ReasoningDelta
	if state.currentStep != nil {
		state.currentStep.Reasoning += ev.ReasoningDelta
		appendStepReasoning(state.currentStep, ev.ReasoningDelta, ev.ThoughtSignature)
	}
}

func appendStepText(step *StepOutput, text, signature string) {
	if text == "" {
		return
	}
	if n := len(step.Content); n > 0 &&
		step.Content[n-1].Type == ContentPartTypeText &&
		step.Content[n-1].ThoughtSignature == signature {
		step.Content[n-1].Text += text
		return
	}
	step.Content = append(step.Content, ContentPart{Type: ContentPartTypeText, Text: text, ThoughtSignature: signature})
}

func appendStepReasoning(step *StepOutput, reasoning, signature string) {
	if reasoning == "" {
		return
	}
	if n := len(step.Content); n > 0 &&
		step.Content[n-1].Type == ContentPartTypeReasoning &&
		step.Content[n-1].ThoughtSignature == signature {
		step.Content[n-1].ReasoningText += reasoning
		return
	}
	step.Content = append(step.Content, ContentPart{
		Type:             ContentPartTypeReasoning,
		ReasoningText:    reasoning,
		ThoughtSignature: signature,
	})
}

func (state *consumeState) consumeToolResult(ev StepEvent) {
	if state.currentStep == nil && ev.ToolResult != nil {
		state.preludeToolResults = append(state.preludeToolResults, *ev.ToolResult)
	}
	if ev.ToolResult != nil {
		state.result.ToolApprovalRequests = removeResolvedApprovalRequest(
			state.result.ToolApprovalRequests, ev.ToolResult.ID,
		)
	}
	handleToolResult(ev, state.result, state.currentStep)
}

func (state *consumeState) consumeApprovalRequest(ev StepEvent) {
	handleToolApprovalRequest(ev, state.currentStep)
	state.result.ToolApprovalRequests = append(state.result.ToolApprovalRequests, ToolApprovalRequest{
		ApprovalID: ev.ApprovalID, ToolCallID: ev.ToolCallID, ToolName: ev.ToolCallName,
		Args: json.RawMessage(ev.ToolCallArgsDelta), Signature: ev.ApprovalSignature,
	})
}

func handleToolApprovalRequest(event StepEvent, step *StepOutput) {
	if step == nil {
		return
	}
	for i := range step.ToolCalls {
		if step.ToolCalls[i].ID == event.ToolCallID {
			step.ToolCalls[i].ApprovalID = event.ApprovalID
			step.ToolCalls[i].ApprovalSignature = event.ApprovalSignature
			for j := range step.Content {
				part := &step.Content[j]
				if part.Type == ContentPartTypeToolCall && part.ToolCallID == event.ToolCallID {
					part.ToolApprovalID = event.ApprovalID
					part.ToolApprovalSignature = event.ApprovalSignature
					break
				}
			}
			return
		}
	}
}

func removeResolvedApprovalRequest(
	requests []ToolApprovalRequest,
	toolCallID string,
) []ToolApprovalRequest {
	for i, request := range requests {
		if request.ToolCallID == toolCallID {
			return append(requests[:i], requests[i+1:]...)
		}
	}
	return requests
}

func responseMessagesWithPrelude(
	prelude []ToolResult,
	steps []StepOutput,
	tools *ToolSet,
) []Message {
	messages := make([]Message, 0, len(prelude)+len(steps))
	for _, result := range prelude {
		part := ToolResultPart(result.ID, result.Name, responseMessageToolOutput(result, tools))
		part.ToolResultApprovalSignature = result.ApprovalSignature
		part.ToolResultApprovalApproved = result.ApprovalApproved
		part.ToolApprovalID = result.ApprovalID
		messages = append(messages, Message{
			Role:    RoleTool,
			Content: []ContentPart{part},
		})
	}
	return append(messages, ResponseMessagesForSteps(steps, tools)...)
}

func handleToolCallStart(event StepEvent, step *StepOutput) {
	if step == nil {
		return
	}
	step.ToolCalls = append(step.ToolCalls, ToolCallOutput{
		ID:               event.ToolCallID,
		Name:             event.ToolCallName,
		Args:             json.RawMessage(event.ToolCallArgsDelta),
		ThoughtSignature: event.ThoughtSignature,
	})
	step.Content = append(step.Content, ContentPart{
		Type: ContentPartTypeToolCall, ToolCallID: event.ToolCallID,
		ToolCallName: event.ToolCallName, ToolCallArgs: json.RawMessage(event.ToolCallArgsDelta),
		ThoughtSignature: event.ThoughtSignature,
	})
}

func handleToolCallDelta(event StepEvent, step *StepOutput) {
	if step == nil || event.ToolCallArgsDelta == "" {
		return
	}
	for i := range step.ToolCalls {
		if step.ToolCalls[i].ID == event.ToolCallID {
			step.ToolCalls[i].Args = append(step.ToolCalls[i].Args, event.ToolCallArgsDelta...)
			break
		}
	}
	for i := range step.Content {
		if step.Content[i].Type == ContentPartTypeToolCall && step.Content[i].ToolCallID == event.ToolCallID {
			step.Content[i].ToolCallArgs = append(step.Content[i].ToolCallArgs, event.ToolCallArgsDelta...)
			break
		}
	}
}

func handleToolCallReady(event StepEvent, step *StepOutput) {
	if step == nil {
		return
	}
	for i := range step.ToolCalls {
		if step.ToolCalls[i].ID == event.ToolCallID {
			step.ToolCalls[i].Name = event.ToolCallName
			if event.ToolCallArgsDelta != "" {
				step.ToolCalls[i].Args = json.RawMessage(event.ToolCallArgsDelta)
			}
			if event.ThoughtSignature != "" {
				step.ToolCalls[i].ThoughtSignature = event.ThoughtSignature
			}
			for j := range step.Content {
				part := &step.Content[j]
				if part.Type != ContentPartTypeToolCall || part.ToolCallID != event.ToolCallID {
					continue
				}
				part.ToolCallName = event.ToolCallName
				if event.ToolCallArgsDelta != "" {
					part.ToolCallArgs = json.RawMessage(event.ToolCallArgsDelta)
				}
				if event.ThoughtSignature != "" {
					part.ThoughtSignature = event.ThoughtSignature
				}
				break
			}
			return
		}
	}
	handleToolCallStart(event, step)
}

func handleToolResult(event StepEvent, result *GenerateTextResult, step *StepOutput) {
	if event.ToolResult == nil {
		return
	}
	result.ToolResults = append(result.ToolResults, *event.ToolResult)
	if step != nil {
		for i := range step.ToolCalls {
			if step.ToolCalls[i].ID == event.ToolResult.ID {
				step.ToolCalls[i].Args = json.RawMessage(event.ToolResult.Args)
				if event.ToolResult.ApprovalID != "" {
					step.ToolCalls[i].ApprovalID = event.ToolResult.ApprovalID
					step.ToolCalls[i].ApprovalSignature = event.ToolResult.ApprovalRequestSignature
				}
				break
			}
		}
		for i := range step.Content {
			part := &step.Content[i]
			if part.Type == ContentPartTypeToolCall && part.ToolCallID == event.ToolResult.ID {
				part.ToolCallArgs = json.RawMessage(event.ToolResult.Args)
				if event.ToolResult.ApprovalID != "" {
					part.ToolApprovalID = event.ToolResult.ApprovalID
					part.ToolApprovalSignature = event.ToolResult.ApprovalRequestSignature
				}
				break
			}
		}
		step.ToolResults = append(step.ToolResults, *event.ToolResult)
	}
}

func handleUsage(
	event StepEvent,
	result *GenerateTextResult,
	step *StepOutput,
	current *Usage,
) {
	if event.Usage == nil {
		return
	}
	result.Usage.InputTokens += event.Usage.InputTokens - current.InputTokens
	result.Usage.InputTokenDetails.NoCacheTokens += event.Usage.InputTokenDetails.NoCacheTokens - current.InputTokenDetails.NoCacheTokens
	result.Usage.InputTokenDetails.CacheReadTokens += event.Usage.InputTokenDetails.CacheReadTokens - current.InputTokenDetails.CacheReadTokens
	result.Usage.InputTokenDetails.CacheWriteTokens += event.Usage.InputTokenDetails.CacheWriteTokens - current.InputTokenDetails.CacheWriteTokens
	result.Usage.OutputTokens += event.Usage.OutputTokens - current.OutputTokens
	result.Usage.OutputTokenDetails.TextTokens += event.Usage.OutputTokenDetails.TextTokens - current.OutputTokenDetails.TextTokens
	result.Usage.OutputTokenDetails.ReasoningTokens += event.Usage.OutputTokenDetails.ReasoningTokens - current.OutputTokenDetails.ReasoningTokens
	result.Usage.TotalTokens += event.Usage.TotalTokens - current.TotalTokens
	if event.Usage.Raw != nil {
		result.Usage.Raw = snapshotJSONMap(event.Usage.Raw)
	}
	*current = *snapshotUsage(event.Usage)
	if step != nil {
		step.Usage = *current
	}
}

func handleSource(event StepEvent, result *GenerateTextResult, step *StepOutput) {
	if event.Source == nil {
		return
	}
	result.Sources = append(result.Sources, *event.Source)
	if step != nil {
		step.Sources = append(step.Sources, *event.Source)
	}
}

func handleFileDelta(event StepEvent, result *GenerateTextResult, step *StepOutput) {
	if len(event.FileData) == 0 {
		return
	}
	file := GeneratedFile{Data: append([]byte(nil), event.FileData...), MediaType: event.FileMediaType}
	result.Files = append(result.Files, file)
	if step != nil {
		step.Files = append(step.Files, GeneratedFile{
			Data: append([]byte(nil), file.Data...), MediaType: file.MediaType,
		})
		step.Content = append(step.Content, ContentPart{
			Type: ContentPartTypeFile, Data: append([]byte(nil), event.FileData...),
			MediaType: event.FileMediaType,
		})
	}
}

func handleStepEnd(
	event StepEvent,
	result *GenerateTextResult,
	step *StepOutput,
	tools *ToolSet,
) {
	if step == nil {
		return
	}
	step.FinishReason = event.FinishReason
	step.RawFinishReason = event.RawFinishReason
	step.ProviderMetadata = event.ProviderMetadata
	step.Warnings = event.Warnings
	step.Response = Response{Messages: ResponseMessagesForStep(*step, tools)}
	result.Steps = append(result.Steps, *step)
	result.FinalStep = *step
	result.Text = step.Text
	result.Reasoning = step.Reasoning
	result.FinishReason = event.FinishReason
	result.RawFinishReason = event.RawFinishReason
	result.ProviderMetadata = event.ProviderMetadata
	result.Warnings = append(result.Warnings, step.Warnings...)
}
