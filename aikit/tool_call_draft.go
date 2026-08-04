package aikit

import (
	"encoding/json"
	"slices"
	"strings"
)

// ToolCallDraft is one tool call assembled from streaming deltas.
type ToolCallDraft struct {
	Index            int
	ID               string
	Name             string
	Args             string
	ThoughtSignature string
	// Complete reports that Args already parses as valid JSON. Further argument
	// deltas for the same index are ignored once it is set.
	Complete bool
}

// ToolCallFold assembles streaming tool-call deltas into drafts keyed by
// StreamEvent.ToolCallIndex. It is single-goroutine-owned and holds no
// emission concerns: a caller that must also forward events observes the
// isNew result of Add.
//
// Three rules resolve provider disagreement, each chosen against the in-repo
// decoders:
//
//   - Arguments stop accumulating once they parse as valid JSON. A provider
//     that re-sends complete arguments would otherwise concatenate them into
//     invalid JSON and fail the call. Incrementally streamed JSON is not valid
//     until its closing token, so the gate cannot fire early on it.
//   - The first non-empty thought signature wins. The signature belongs to the
//     call as announced.
//   - A non-empty ID or name on a later delta overwrites the current one.
//     Providers that send the tool name in a later chunk would otherwise lose
//     it; providers that send both only in the first delta are unaffected.
type ToolCallFold struct {
	drafts map[int]*foldedToolCall
}

// foldedToolCall accumulates one index. Arguments go into a builder rather than
// a string so appending n deltas stays linear, and a structural scan of each
// delta gates the full json.Valid pass: unbalanced brackets or an open string
// cannot be valid JSON, so the expensive whole-buffer check runs only when the
// accumulated text could plausibly parse. Both are optimizations — the deltas
// on which Complete flips are exactly the ones a json.Valid-per-delta fold
// would pick.
type foldedToolCall struct {
	draft    ToolCallDraft
	args     strings.Builder
	depth    int
	inString bool
	escaped  bool
	// validations counts whole-buffer json.Valid passes. Each one is O(len),
	// so the gate is only doing its job while this stays small relative to the
	// delta count; a test asserts that.
	validations int
}

// Add integrates one StreamEventToolCallDelta and reports whether it opened a
// previously unseen tool-call index.
func (f *ToolCallFold) Add(event StreamEvent) (isNew bool) {
	if f.drafts == nil {
		f.drafts = make(map[int]*foldedToolCall)
	}
	folded, exists := f.drafts[event.ToolCallIndex]
	if !exists {
		folded = &foldedToolCall{draft: ToolCallDraft{
			Index:            event.ToolCallIndex,
			ID:               event.ToolCallID,
			Name:             event.ToolCallName,
			ThoughtSignature: event.ThoughtSignature,
		}}
		f.drafts[event.ToolCallIndex] = folded
		folded.appendArgs(event.ToolCallArgsDelta)
		return true
	}
	if event.ToolCallID != "" {
		folded.draft.ID = event.ToolCallID
	}
	if event.ToolCallName != "" {
		folded.draft.Name = event.ToolCallName
	}
	if event.ThoughtSignature != "" && folded.draft.ThoughtSignature == "" {
		folded.draft.ThoughtSignature = event.ThoughtSignature
	}
	folded.appendArgs(event.ToolCallArgsDelta)
	return false
}

// Completed returns every draft ordered by tool-call index.
func (f *ToolCallFold) Completed() []ToolCallDraft {
	if len(f.drafts) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(f.drafts))
	for index := range f.drafts {
		indexes = append(indexes, index)
	}
	slices.Sort(indexes)
	drafts := make([]ToolCallDraft, 0, len(indexes))
	for _, index := range indexes {
		folded := f.drafts[index]
		draft := folded.draft
		draft.Args = folded.args.String()
		drafts = append(drafts, draft)
	}
	return drafts
}

// Len reports how many distinct tool-call indexes have been seen.
func (f *ToolCallFold) Len() int { return len(f.drafts) }

func (c *foldedToolCall) appendArgs(delta string) {
	if delta == "" || c.draft.Complete {
		return
	}
	c.args.WriteString(delta)
	if !c.scanBalances(delta) {
		return
	}
	c.validations++
	c.draft.Complete = json.Valid([]byte(c.args.String()))
}

// scanBalances advances the structural state over delta and reports whether the
// accumulated arguments could now be a complete JSON value. An open string or
// an unclosed bracket rules validity out without reading the whole buffer.
func (c *foldedToolCall) scanBalances(delta string) bool {
	for i := 0; i < len(delta); i++ {
		char := delta[i]
		switch {
		case c.escaped:
			c.escaped = false
		case c.inString && char == '\\':
			c.escaped = true
		case char == '"':
			c.inString = !c.inString
		case c.inString:
		case char == '{' || char == '[':
			c.depth++
		case char == '}' || char == ']':
			c.depth--
		}
	}
	return c.depth == 0 && !c.inString && !c.escaped
}
