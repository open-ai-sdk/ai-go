package ainode

import (
	"encoding/json"
	"strings"
)

func (b *PersistedMessageBuilder) Content() string {
	var content strings.Builder
	for _, raw := range b.parts {
		part, ok := raw.(map[string]any)
		if !ok || part["type"] != "text" {
			continue
		}
		text, ok := part["text"].(string)
		if !ok {
			continue
		}
		content.WriteString(text)
	}
	return content.String()
}

func (b *PersistedMessageBuilder) Parts() json.RawMessage {
	if len(b.parts) == 0 {
		return nil
	}
	parts, err := json.Marshal(b.parts)
	if err != nil {
		return nil
	}
	return parts
}

func (b *PersistedMessageBuilder) Metadata() json.RawMessage { return b.metadata }

func MergeWithPersistence(builder *PersistedMessageBuilder) MergeOption {
	return func(config *mergeConfig) { config.persistenceBuilder = builder }
}
