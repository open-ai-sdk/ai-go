package aikit

import "testing"

func TestAppendTextExtendsTrailingPartWithSameSignature(t *testing.T) {
	parts := AppendText(nil, "Hel", "sig")
	parts = AppendText(parts, "lo", "sig")

	if len(parts) != 1 {
		t.Fatalf("len(parts) = %d, want 1", len(parts))
	}
	if parts[0].Type != ContentPartTypeText {
		t.Errorf("Type = %v, want text", parts[0].Type)
	}
	if parts[0].Text != "Hello" {
		t.Errorf("Text = %q, want Hello", parts[0].Text)
	}
	if parts[0].ThoughtSignature != "sig" {
		t.Errorf("ThoughtSignature = %q, want sig", parts[0].ThoughtSignature)
	}
}

func TestAppendTextStartsNewPartOnSignatureChange(t *testing.T) {
	parts := AppendText(nil, "a", "first")
	parts = AppendText(parts, "b", "second")

	if len(parts) != 2 {
		t.Fatalf("len(parts) = %d, want 2", len(parts))
	}
	if parts[0].Text != "a" || parts[1].Text != "b" {
		t.Errorf("parts = (%q, %q), want (a, b)", parts[0].Text, parts[1].Text)
	}
}

func TestAppendTextStartsNewPartAfterAnotherPartType(t *testing.T) {
	parts := AppendText(nil, "a", "")
	parts = AppendReasoning(parts, "why", "")
	parts = AppendText(parts, "b", "")

	if len(parts) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(parts))
	}
	if parts[2].Type != ContentPartTypeText || parts[2].Text != "b" {
		t.Errorf("trailing part = %+v, want a text part holding b", parts[2])
	}
}

func TestAppendTextIgnoresEmptyValue(t *testing.T) {
	if got := AppendText(nil, "", "sig"); got != nil {
		t.Errorf("AppendText(nil, \"\") = %+v, want nil", got)
	}
	parts := AppendText(nil, "a", "")
	if got := AppendText(parts, "", ""); len(got) != 1 || got[0].Text != "a" {
		t.Errorf("empty value changed parts: %+v", got)
	}
}

func TestAppendReasoningExtendsTrailingPartWithSameSignature(t *testing.T) {
	parts := AppendReasoning(nil, "think", "sig")
	parts = AppendReasoning(parts, "ing", "sig")

	if len(parts) != 1 {
		t.Fatalf("len(parts) = %d, want 1", len(parts))
	}
	if parts[0].Type != ContentPartTypeReasoning {
		t.Errorf("Type = %v, want reasoning", parts[0].Type)
	}
	if parts[0].ReasoningText != "thinking" {
		t.Errorf("ReasoningText = %q, want thinking", parts[0].ReasoningText)
	}
}

func TestAppendReasoningIgnoresEmptyValue(t *testing.T) {
	if got := AppendReasoning(nil, "", ""); got != nil {
		t.Errorf("AppendReasoning(nil, \"\") = %+v, want nil", got)
	}
}
