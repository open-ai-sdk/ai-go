package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

func attrMap(attrs []tracing.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value
	}
	return out
}

func TestModelCallAttrs(t *testing.T) {
	tests := []struct {
		name string
		sr   streamResult
		want map[string]any
	}{
		{
			name: "empty result reports nothing",
			sr:   streamResult{},
			want: map[string]any{},
		},
		{
			name: "normalized finish reason without usage",
			sr:   streamResult{finish: FinishReasonStop},
			want: map[string]any{attrGenAIFinishReasons: []string{"stop"}},
		},
		{
			name: "raw finish reason wins",
			sr:   streamResult{finish: FinishReasonToolCalls, rawFinish: "function_call"},
			want: map[string]any{attrGenAIFinishReasons: []string{"function_call"}},
		},
		{
			name: "response id and model from provider metadata",
			sr: streamResult{
				finish: FinishReasonStop,
				providerMeta: map[string]any{
					"alpha": map[string]any{"responseId": "resp_1", "model": "model-2026-01"},
					"other": map[string]any{"responseId": "ignored"},
				},
			},
			want: map[string]any{
				attrGenAIFinishReasons: []string{"stop"},
				attrGenAIResponseID:    "resp_1",
				attrGenAIResponseModel: "model-2026-01",
			},
		},
		{
			name: "top level keys and string maps are read",
			sr: streamResult{
				providerMeta: map[string]any{
					"responseId": "top",
					"vendor":     map[string]string{"model": "typed-map"},
				},
			},
			want: map[string]any{attrGenAIResponseID: "top", attrGenAIResponseModel: "typed-map"},
		},
		{
			name: "non-string and empty values are skipped",
			sr: streamResult{providerMeta: map[string]any{
				"vendor": map[string]any{"responseId": 7, "model": ""},
			}},
			want: map[string]any{},
		},
		{
			name: "usage with cache and reasoning",
			sr: streamResult{usage: &Usage{
				InputTokens:        100,
				OutputTokens:       40,
				InputTokenDetails:  InputTokenDetails{CacheReadTokens: 60, CacheWriteTokens: 5},
				OutputTokenDetails: OutputTokenDetails{ReasoningTokens: 12},
			}},
			want: map[string]any{
				attrGenAIUsageInput:      100,
				attrGenAIUsageOutput:     40,
				attrGenAIUsageCacheRead:  60,
				attrGenAIUsageCacheWrite: 5,
				attrGenAIUsageReasoning:  12,
			},
		},
		{
			name: "zero detail counters are omitted",
			sr:   streamResult{usage: &Usage{InputTokens: 3, OutputTokens: 1}},
			want: map[string]any{attrGenAIUsageInput: 3, attrGenAIUsageOutput: 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := attrMap(modelCallAttrs(test.sr))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("modelCallAttrs() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestModelCallSpanCarriesGenAIAttributesAndKeepsAIAttributes(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{{
		textEvt("done"),
		{Type: StreamEventUsage, Usage: &Usage{
			InputTokens: 10, OutputTokens: 4, TotalTokens: 14,
			InputTokenDetails:  InputTokenDetails{CacheReadTokens: 6},
			OutputTokenDetails: OutputTokenDetails{ReasoningTokens: 2},
		}},
		{
			Type:             StreamEventFinish,
			FinishReason:     FinishReasonStop,
			ProviderMetadata: map[string]any{"vendor": map[string]any{"responseId": "resp_9", "model": "m-snapshot"}},
		},
	}}}
	ft := &fakeTracer{}
	for ev := range driveStream(context.Background(), runConfig{Model: model, Tracer: ft, MaxSteps: 1}) {
		if ev.Type == StepEventError {
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}
	spans := ft.byName("ai.model_call")
	if len(spans) != 1 {
		t.Fatalf("model spans = %d, want 1", len(spans))
	}
	attrs := attrMap(spans[0].attrs)
	want := map[string]any{
		attrGenAIRequestModel:    model.ModelID(),
		attrGenAIResponseID:      "resp_9",
		attrGenAIResponseModel:   "m-snapshot",
		attrGenAIUsageInput:      10,
		attrGenAIUsageOutput:     4,
		attrGenAIUsageCacheRead:  6,
		attrGenAIUsageReasoning:  2,
		"ai.usage.total_tokens":  14,
		"ai.usage.input_tokens":  10,
		"ai.finish_reason":       "stop",
		"ai.model_id":            model.ModelID(),
		attrGenAIFinishReasons:   []string{"stop"},
		"ai.usage.output_tokens": 4,
		"ai.step_number":         0,
	}
	for key, value := range want {
		if got, ok := attrs[key]; !ok || !reflect.DeepEqual(got, value) {
			t.Errorf("model span attr %q = %#v (present=%v), want %#v", key, got, ok, value)
		}
	}
	for _, step := range ft.byName("ai.step") {
		if hasAttrKey(step.attrs, attrGenAIRequestModel) || hasAttrKey(step.attrs, attrGenAIResponseID) {
			t.Errorf("step span must not carry model-call gen_ai attributes: %v", step.attrs)
		}
	}
}
