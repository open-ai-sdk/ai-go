package ainode_test

import "github.com/open-ai-sdk/ai-go/aisdk"

// Keep the original two-field shape compatible with external unkeyed literals.
var _ = aisdk.Chunk{"text-delta", map[string]any{"delta": "hello"}}
