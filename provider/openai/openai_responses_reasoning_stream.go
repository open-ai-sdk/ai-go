package openai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Reasoning is decoded only when explicitly enabled. In particular, source IDs
// are pointers: an absent ID is not the same as an explicitly present empty ID.
type responsesReasoningPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesReasoningItem struct {
	Type    string                   `json:"type"`
	ID      *string                  `json:"id"`
	Summary []responsesReasoningPart `json:"summary"`
}

type responsesReasoningChunk struct {
	Item         *responsesReasoningItem `json:"item"`
	ItemID       *string                 `json:"item_id"`
	OutputIndex  *int                    `json:"output_index"`
	SummaryIndex int                     `json:"summary_index"`
	Delta        string                  `json:"delta"`
	Text         string                  `json:"text"`
	Part         *responsesReasoningPart `json:"part"`
	Response     *struct {
		Output []responsesReasoningItem `json:"output"`
	} `json:"response"`
}

type pendingReasoningPart struct {
	kind  string
	delta strings.Builder
	text  *string
}

type pendingReasoning struct {
	parts map[int]*pendingReasoningPart
	done  bool
}

type responsesReasoningState struct {
	byID    map[string]*pendingReasoning
	byIndex map[int]*pendingReasoning
	current *pendingReasoning
}

func newResponsesReasoningState() *responsesReasoningState {
	return &responsesReasoningState{
		byID:    make(map[string]*pendingReasoning),
		byIndex: make(map[int]*pendingReasoning),
	}
}

func (s *responsesReasoningState) item(id *string, index *int, added bool) *pendingReasoning {
	var item *pendingReasoning
	if id != nil {
		item = s.byID[*id]
	}
	if item == nil && index != nil {
		item = s.byIndex[*index]
	}
	if item == nil && id == nil && index == nil && !added {
		item = s.current
	}
	if item == nil {
		item = &pendingReasoning{}
	}
	if id != nil {
		s.byID[*id] = item
	}
	if index != nil {
		s.byIndex[*index] = item
	}
	s.current = item
	return item
}

func (p *pendingReasoning) part(index int) *pendingReasoningPart {
	if p.parts == nil {
		p.parts = make(map[int]*pendingReasoningPart)
	}
	part := p.parts[index]
	if part == nil {
		part = &pendingReasoningPart{kind: "summary_text"}
		p.parts[index] = part
	}
	return part
}

// dispatch returns handled only for deltas, replacing the legacy delta emission
// with an otherwise identical event carrying its raw source item_id. Summary
// part completion is not item completion; only an actual completed item emits
// a terminal summary. A completed response's output can supply such an item
// when the endpoint omitted its individual output_item.done event.
func (s *responsesReasoningState) dispatch(data, kind string, ch chan<- aikit.StreamEvent) (bool, error) {
	switch kind {
	case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
		"response.output_item.added", "response.output_item.done", "response.completed":
	default:
		return false, nil
	}
	var chunk responsesReasoningChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false, fmt.Errorf("openai: unmarshal reasoning chunk: %w", err)
	}
	switch kind {
	case "response.output_item.added", "response.output_item.done":
		if chunk.Item == nil || chunk.Item.Type != "reasoning" {
			return false, nil
		}
		item := s.item(chunk.Item.ID, chunk.OutputIndex, kind == "response.output_item.added")
		if kind == "response.output_item.done" {
			item.finish(*chunk.Item, ch)
		} else if !item.done {
			for index, summary := range chunk.Item.Summary {
				part := item.part(index)
				part.kind = summary.Type
				part.delta.WriteString(summary.Text)
			}
		}
	case "response.completed":
		if chunk.Response != nil {
			for index, output := range chunk.Response.Output {
				if output.Type == "reasoning" {
					s.item(output.ID, &index, false).finish(output, ch)
				}
			}
		}
	default:
		return s.dispatchPart(kind, chunk, ch), nil
	}
	return false, nil
}

func (s *responsesReasoningState) dispatchPart(
	kind string,
	chunk responsesReasoningChunk,
	ch chan<- aikit.StreamEvent,
) bool {
	item := s.item(chunk.ItemID, chunk.OutputIndex, false)
	if !item.done {
		part := item.part(chunk.SummaryIndex)
		switch kind {
		case "response.reasoning_summary_text.delta":
			part.delta.WriteString(chunk.Delta)
		case "response.reasoning_summary_text.done":
			part.text = &chunk.Text
		case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
			if chunk.Part != nil {
				part.kind = chunk.Part.Type
				if kind == "response.reasoning_summary_part.done" {
					part.text = &chunk.Part.Text
				} else if part.delta.Len() == 0 {
					part.delta.WriteString(chunk.Part.Text)
				}
			}
		}
	}
	if kind != "response.reasoning_summary_text.delta" {
		return false
	}
	if chunk.Delta != "" {
		ch <- aikit.StreamEvent{
			Type: aikit.StreamEventReasoningDelta, TextDelta: chunk.Delta,
			ReasoningID: chunk.ItemID,
		}
	}
	return true
}

func (p *pendingReasoning) finish(item responsesReasoningItem, ch chan<- aikit.StreamEvent) {
	if p.done {
		return
	}
	p.done = true
	var summary []string
	appendSummary := func(kind, text string) {
		if kind == "summary_text" {
			if text = strings.TrimSpace(text); text != "" {
				summary = append(summary, text)
			}
		}
	}
	if item.Summary != nil {
		// The final item's summary is authoritative, even when explicitly empty.
		for _, part := range item.Summary {
			appendSummary(part.Type, part.Text)
		}
	} else {
		indices := make([]int, 0, len(p.parts))
		for index := range p.parts {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			part := p.parts[index]
			text := part.delta.String()
			if part.text != nil {
				text = *part.text
			}
			appendSummary(part.kind, text)
		}
	}
	ch <- aikit.StreamEvent{
		Type: aikit.StreamEventReasoningSummary, ReasoningID: item.ID,
		ReasoningSummary: strings.Join(summary, "\n\n"),
	}
	// Keep the completion marker for deduplication, not retained summary data.
	p.parts = nil
}
