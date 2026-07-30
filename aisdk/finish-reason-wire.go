package aisdk

// WireFinishReason is the `finish.finishReason` value as the AI SDK v7 client
// accepts it. The client validates it against a closed enum
// (ui-message-chunks.ts, the `finish` member), and the transport does
// `if (!chunk.success) throw chunk.error`, so an out-of-enum value is not a
// cosmetic difference — it throws in the browser.
type WireFinishReason string

const (
	WireFinishStop          WireFinishReason = "stop"
	WireFinishLength        WireFinishReason = "length"
	WireFinishContentFilter WireFinishReason = "content-filter"
	WireFinishToolCalls     WireFinishReason = "tool-calls"
	WireFinishError         WireFinishReason = "error"
	WireFinishOther         WireFinishReason = "other"
)

// wireFinishReasons is the exact accepted set, used to validate anything arriving
// as a bare string.
var wireFinishReasons = map[WireFinishReason]struct{}{
	WireFinishStop: {}, WireFinishLength: {}, WireFinishContentFilter: {},
	WireFinishToolCalls: {}, WireFinishError: {}, WireFinishOther: {},
}

// ToWireFinishReason maps the internal FinishReason vocabulary onto the wire enum.
//
// The two vocabularies genuinely differ, and not only in spelling: the internal set
// uses underscores (tool_calls, content_filter) and has `unknown`, while the wire set
// uses hyphens and has `other`. Passing an internal value through unmapped emits
// finishReason:"tool_calls", which fails the client's enum — so every tool-calling
// conversation would throw. Anything unrecognised degrades to `other` rather than
// emitting an invalid value.
func ToWireFinishReason(fr FinishReason) WireFinishReason {
	switch fr {
	case FinishReasonStop:
		return WireFinishStop
	case FinishReasonLength:
		return WireFinishLength
	case FinishReasonContentFilter:
		return WireFinishContentFilter
	case FinishReasonToolCalls:
		return WireFinishToolCalls
	case FinishReasonError:
		return WireFinishError
	default:
		// Covers FinishReasonUnknown, which has no wire counterpart, and any
		// value a provider adds later.
		return WireFinishOther
	}
}

// NormalizeWireFinishReason accepts a finish reason that is already a wire string
// and passes it through, mapping an internal spelling or an unknown value onto the
// enum. It exists because several call paths carry the reason as a bare string.
//
// The empty string is returned unchanged: `finishReason` is optional on the wire, and
// omitting it is valid, whereas coercing "" to "other" would assert a reason nobody
// reported.
func NormalizeWireFinishReason(s string) WireFinishReason {
	if s == "" {
		return ""
	}
	if _, ok := wireFinishReasons[WireFinishReason(s)]; ok {
		return WireFinishReason(s)
	}
	return ToWireFinishReason(FinishReason(s))
}
