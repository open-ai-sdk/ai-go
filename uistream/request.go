package uistream

import "github.com/open-ai-sdk/ai-go/aikit"

// Request is the protocol-neutral representation of an inbound UI request.
type Request struct {
	Messages               []aikit.Message
	MessageID, ID, Trigger string
	Body, Metadata, Extra  map[string]any
}
