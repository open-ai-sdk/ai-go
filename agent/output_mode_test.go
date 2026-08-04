package agent

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResolveOutputMode(t *testing.T) {
	tests := []struct {
		name                          string
		requested                     OutputMode
		hasSchema, hasTools, callable bool
		native                        llm.NativeSchemaSupport
		want                          OutputMode
		wantErr                       bool
	}{
		{"no schema", OutputModeAuto, false, true, false, llm.NativeSchemaNone, OutputModeNative, false},
		{"openai auto", OutputModeAuto, true, true, true, llm.NativeSchemaFull, OutputModeNative, false},
		{
			"gemini native with tools",
			OutputModeAuto,
			true,
			true,
			true,
			llm.NativeSchemaSuppressesTools,
			OutputModeTool,
			false,
		},
		{
			"gemini native alone",
			OutputModeAuto,
			true,
			false,
			false,
			llm.NativeSchemaSuppressesTools,
			OutputModeNative,
			false,
		},
		{"forced native rejected", OutputModeNative, true, true, true, llm.NativeSchemaSuppressesTools, "", true},
		{"forced tool rejected", OutputModeTool, true, false, false, llm.NativeSchemaFull, "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveOutputMode(test.requested, test.hasSchema, test.hasTools, test.callable, test.native)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("resolveOutputMode() = (%q, %v), want (%q, err=%v)", got, err, test.want, test.wantErr)
			}
		})
	}
}
