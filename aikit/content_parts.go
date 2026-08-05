package aikit

// AppendText appends value to parts as assistant text, extending the trailing
// text part when it carries the same thought signature. A differing signature
// starts a new part so provider-attributed segments stay separable. An empty
// value is ignored.
func AppendText(parts []ContentPart, value, signature string) []ContentPart {
	if value == "" {
		return parts
	}
	if n := len(parts); n > 0 &&
		parts[n-1].Type == ContentPartTypeText &&
		parts[n-1].ThoughtSignature == signature {
		parts[n-1].Text += value
		return parts
	}
	return append(parts, ContentPart{
		Type: ContentPartTypeText, Text: value, ThoughtSignature: signature,
	})
}

// AppendReasoning appends value to parts as reasoning text under the same
// trailing-part and signature rules as AppendText.
func AppendReasoning(parts []ContentPart, value, signature string) []ContentPart {
	if value == "" {
		return parts
	}
	if n := len(parts); n > 0 &&
		parts[n-1].Type == ContentPartTypeReasoning &&
		parts[n-1].ThoughtSignature == signature {
		parts[n-1].ReasoningText += value
		return parts
	}
	return append(parts, ContentPart{
		Type: ContentPartTypeReasoning, ReasoningText: value, ThoughtSignature: signature,
	})
}
