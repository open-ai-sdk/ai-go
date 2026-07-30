package einoadapter

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// The agentic API this adapter is built on does not exist below eino v0.9.0-beta.1:
// schema/agentic_message.go is absent, and every ref/eino line number cited in the
// plan was read at v0.9.13. A silently downgraded pin would otherwise surface as a
// pile of confusing errors in later phases, so the four types the adapter actually
// depends on are named here. Dropping below the pin breaks this file first.
//
// The CI shell check (`go list -m github.com/cloudwego/eino`) asserts the exact
// version; this asserts the shape, which is what the code depends on.
var (
	// Block-shaped messages — the reason Eino was chosen over the flat path.
	_ *schema.AgenticMessage = nil
	_ *schema.ContentBlock   = nil
	// Per-index streaming metadata: the only identity a delta frame carries.
	_ *schema.StreamingMeta = nil
	// The agentic event type the adapter converts from.
	_ *adk.TypedAgentEvent[*schema.AgenticMessage] = nil
	// The approval gate's seam: one interface carrying all four tool-wrapping
	// hooks plus the after-model batch pre-gate.
	_ adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage] = nil
	// Sibling outputs survive an interrupt through this; a plain error discards them.
	_ *compose.ToolsInterruptAndRerunExtra = nil
)
