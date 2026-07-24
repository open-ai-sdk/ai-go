package engine

import (
	"sync"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

func executeToolCalls(
	r *run,
	tools *ToolSet,
	prepared []preparedToolCall,
	history *[]Message,
	approval map[string]func(string, string) bool, approver ApprovalResponder,
) (toolNames []string, stepToolCalls []ToolCallInfo, stepToolResults []ToolResult) {
	toolNames = make([]string, 0, len(prepared))
	for _, preparedCall := range prepared {
		tc := preparedCall.tc
		if preparedCall.invalidErr != nil {
			r.emit(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        tc.id,
				ToolCallName:      tc.name,
				ToolCallArgsDelta: tc.args,
			})
			errOutput := invalidToolCallOutput(tc, preparedCall.invalidErr)
			*history = append(*history, buildToolResultMessage(tc.id, tc.name, errOutput))
			toolNames = append(toolNames, tc.name)
			continue
		}

		r.emit(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		})

		result := approvedToolCall(r, tools, tc, approval, approver)

		// Apply ToModelOutput transform for history; event keeps original output.
		modelOutput := result.Output
		if tools != nil {
			for _, def := range tools.Definitions {
				if def.Name == tc.name && def.ToModelOutput != nil {
					modelOutput = def.ToModelOutput(result.Output)
					break
				}
			}
		}

		*history = append(*history, buildToolResultMessage(tc.id, tc.name, modelOutput))
		r.emit(StepEvent{Type: StepEventToolResult, ToolResult: result})
		toolNames = append(toolNames, tc.name)
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:               tc.id,
			Name:             tc.name,
			Args:             tc.args,
			ArgsSet:          true,
			ThoughtSignature: tc.thoughtSignature,
		})
		stepToolResults = append(stepToolResults, *result)
	}
	return toolNames, stepToolCalls, stepToolResults
}

// executeToolCallsParallel processes tool calls concurrently with a semaphore.
func executeToolCallsParallel(
	r *run,
	tools *ToolSet,
	prepared []preparedToolCall,
	history *[]Message,
	maxParallel int,
	approval map[string]func(string, string) bool, approver ApprovalResponder,
) (toolNames []string, stepToolCalls []ToolCallInfo, stepToolResults []ToolResult) {
	if maxParallel <= 0 {
		maxParallel = 5
	}

	type indexedResult struct {
		idx         int
		tc          toolCallState
		result      *ToolResult
		modelOutput string
		valid       bool
		invalidErr  error
	}

	results := make([]indexedResult, len(prepared))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, preparedCall := range prepared {
		tc := preparedCall.tc
		if preparedCall.invalidErr != nil {
			results[i] = indexedResult{
				idx: i, tc: tc, valid: false, invalidErr: preparedCall.invalidErr,
				result: &ToolResult{
					ID: tc.id, Name: tc.name, Args: tc.args,
					Output: invalidToolCallOutput(tc, preparedCall.invalidErr),
				},
			}
			continue
		}

		// Emit ToolCallReady before execution starts (matches sequential contract)
		r.emit(StepEvent{
			Type:              StepEventToolCallReady,
			ToolCallID:        tc.id,
			ToolCallName:      tc.name,
			ToolCallArgsDelta: tc.args,
			ThoughtSignature:  tc.thoughtSignature,
		})

		results[i] = indexedResult{idx: i, tc: tc, valid: true}
		wg.Add(1)
		go func(idx int, tc toolCallState) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// A panic in a tool executor (or ToModelOutput) is contained to this
			// call: it becomes that tool's error result so sibling tools still
			// complete and the model sees the failure, rather than crashing the
			// process. Recover is deferred last so it runs first and populates the
			// result before wg.Done.
			defer safego.Recover(r.logger, func(err error) {
				results[idx].result = &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args, Output: invalidToolCallOutput(tc, err)}
				results[idx].modelOutput = results[idx].result.Output
			}, "phase", "parallel-tool-exec", "tool", tc.name)

			result := approvedToolCall(r, tools, tc, approval, approver)
			modelOutput := result.Output
			if tools != nil {
				for _, def := range tools.Definitions {
					if def.Name == tc.name && def.ToModelOutput != nil {
						modelOutput = def.ToModelOutput(result.Output)
						break
					}
				}
			}
			results[idx].result = result
			results[idx].modelOutput = modelOutput
		}(i, tc)
	}
	wg.Wait()

	// Emit events and build history in original order
	toolNames = make([]string, 0, len(prepared))
	for _, res := range results {
		if !res.valid {
			r.emit(StepEvent{
				Type:              StepEventToolCallInvalid,
				ToolCallID:        res.tc.id,
				ToolCallName:      res.tc.name,
				ToolCallArgsDelta: res.tc.args,
			})
			*history = append(*history, buildToolResultMessage(res.tc.id, res.tc.name, res.result.Output))
			toolNames = append(toolNames, res.tc.name)
			continue
		}

		r.emit(StepEvent{Type: StepEventToolResult, ToolResult: res.result})
		*history = append(*history, buildToolResultMessage(res.tc.id, res.tc.name, res.modelOutput))
		toolNames = append(toolNames, res.tc.name)
		stepToolCalls = append(stepToolCalls, ToolCallInfo{
			ID:               res.tc.id,
			Name:             res.tc.name,
			Args:             res.tc.args,
			ArgsSet:          true,
			ThoughtSignature: res.tc.thoughtSignature,
		})
		stepToolResults = append(stepToolResults, *res.result)
	}
	return toolNames, stepToolCalls, stepToolResults
}

func approvedToolCall(
	r *run,
	tools *ToolSet,
	tc toolCallState,
	approval map[string]func(string, string) bool,
	approver ApprovalResponder,
) *ToolResult {
	if policy := approval[tc.name]; policy != nil && policy(tc.name, tc.args) {
		request := ApprovalRequest{ApprovalID: tc.id, ToolCallID: tc.id, ToolName: tc.name, Args: tc.args}
		r.emit(StepEvent{Type: StepEventToolApprovalRequest, ToolCallID: tc.id, ToolCallName: tc.name, ToolCallArgsDelta: tc.args})
		if approver == nil {
			r.emit(StepEvent{Type: StepEventToolOutputDenied, ToolCallID: tc.id})
			return &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args, Output: `{"error":"tool approval denied"}`}
		}
		response, err := approver.RequestApproval(r.ctx, request)
		if err != nil || !response.Approved {
			r.emit(StepEvent{Type: StepEventToolOutputDenied, ToolCallID: tc.id})
			return &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args, Output: `{"error":"tool approval denied"}`}
		}
	}
	return executeToolCall(r.ctx, tools, tc)
}
