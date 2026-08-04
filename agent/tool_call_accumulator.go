package agent

import "github.com/open-ai-sdk/ai-go/aikit"

// toolCallState is one tool call as the run engine carries it: assembled from
// streaming deltas, then repaired, validated, and executed.
type toolCallState struct {
	id               string
	name             string
	args             string
	thoughtSignature string
}

// toolCallAccumulator groups streaming tool-call deltas by index. Assembly
// itself lives in aikit.ToolCallFold so the agent and direct completions share
// one set of provider-disagreement rules; this type only adapts the drafts to
// the run engine's value type.
type toolCallAccumulator struct {
	fold aikit.ToolCallFold
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{}
}

// add integrates a StreamEvent tool-call delta. Returns true if this is a new index.
func (a *toolCallAccumulator) add(ev StreamEvent) bool {
	return a.fold.Add(ev)
}

// completed returns all accumulated tool calls sorted by index.
func (a *toolCallAccumulator) completed() []toolCallState {
	drafts := a.fold.Completed()
	if len(drafts) == 0 {
		return nil
	}
	states := make([]toolCallState, 0, len(drafts))
	for _, draft := range drafts {
		states = append(states, toolCallState{
			id:               draft.ID,
			name:             draft.Name,
			args:             draft.Args,
			thoughtSignature: draft.ThoughtSignature,
		})
	}
	return states
}

func (a *toolCallAccumulator) hasToolCalls() bool { return a.fold.Len() > 0 }
