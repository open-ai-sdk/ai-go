package agent

import (
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// stepAttrs reports the finish reason and token usage recorded for one model
// call. It never includes prompt or completion text — that is content,
// attached separately and only when the caller opted into WithTraceContent.
func stepAttrs(sr streamResult) []tracing.Attr {
	attrs := []tracing.Attr{{Key: "ai.finish_reason", Value: string(sr.finish)}}
	if sr.usage != nil {
		attrs = append(attrs,
			tracing.Attr{Key: "ai.usage.input_tokens", Value: sr.usage.InputTokens},
			tracing.Attr{Key: "ai.usage.output_tokens", Value: sr.usage.OutputTokens},
			tracing.Attr{Key: "ai.usage.total_tokens", Value: sr.usage.TotalTokens},
		)
	}
	return attrs
}

// marshalMessagesForTrace renders the conversation sent to the model as a
// span attribute value. Only called by sites gated on traceContent — the
// span content policy is enforced by those call sites, not by this helper.
func marshalMessagesForTrace(msgs []Message) string {
	b, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(b)
}
