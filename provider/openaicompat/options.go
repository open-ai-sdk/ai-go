package openaicompat

import "github.com/open-ai-sdk/ai-go/llm"

// CapabilityFlags declares optional features a provider supports.
type CapabilityFlags struct {
	// SupportsStructuredOutput indicates the provider accepts json_schema response_format.
	SupportsStructuredOutput bool
	// NativeSchema reports whether native schema constraints compose with tools.
	NativeSchema llm.NativeSchemaSupport
	// SupportsStreamUsage indicates the provider emits usage in streaming chunks.
	SupportsStreamUsage bool
}
