package openai

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestResponsesInstructionTrimmingIsOptional(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		top, trim         bool
	}{
		{"verbatim-system", " \n\tPolicy\u2003", " \n\tPolicy\u2003", false, false},
		{"trim-system", " \n\tPolicy\u2003", "Policy", false, true},
		{"verbatim-top-level", " \n\tPolicy\u2003", " \n\tPolicy\u2003", true, false},
		{"trim-top-level", " \n\tPolicy\u2003", "Policy", true, true},
		{"omit-blank-system", " \n\t\u2003", "", false, true},
		{"omit-blank-top-level", " \n\t\u2003", "", true, true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.name+"/stream="+strconv.FormatBool(stream), func(t *testing.T) {
				req := llm.Request{
					Instructions: tc.input,
					Messages:     []aikit.Message{aikit.UserMessage("Hello")},
					ProviderOptions: map[string]any{
						"openai": ProviderOptions{InstructionsAsTopLevel: tc.top, TrimInstructions: tc.trim},
					},
				}
				encoded, _, err := encodeRequest("example-model", req, stream)
				if err != nil {
					t.Fatal(err)
				}
				if req.Instructions != tc.input {
					t.Fatal("caller-owned request mutated")
				}
				if tc.top {
					if encoded.Instructions != tc.want || len(encoded.Input) != 1 {
						t.Fatalf("instructions=%q input=%#v", encoded.Instructions, encoded.Input)
					}
					raw, err := json.Marshal(encoded)
					if err != nil {
						t.Fatal(err)
					}
					var body map[string]any
					if err = json.Unmarshal(raw, &body); err != nil {
						t.Fatal(err)
					}
					if _, present := body["instructions"]; present != (tc.want != "") {
						t.Fatal(string(raw))
					}
				} else if tc.want == "" {
					if len(encoded.Input) != 1 || encoded.Input[0].Role != "user" {
						t.Fatal(encoded.Input)
					}
				} else {
					if len(encoded.Input) != 2 || encoded.Input[0].Role != "system" ||
						len(encoded.Input[0].Content) != 1 {
						t.Fatal(encoded.Input)
					}
					part, ok := encoded.Input[0].Content[0].(inputTextPart)
					if !ok || part.Text != tc.want {
						t.Fatal(encoded.Input[0])
					}
				}
			})
		}
	}
}

func TestResponsesTrimInstructionsFromMapAndPromptCacheValidation(t *testing.T) {
	req := llm.Request{
		Instructions: " \nPolicy\t",
		Messages:     []aikit.Message{aikit.UserMessage("Hello")},
		ProviderOptions: map[string]any{
			"openai": map[string]any{"instructionsAsTopLevel": true, "trimInstructions": true},
		},
	}
	encoded, _, err := encodeRequest("example-model", req, false)
	if err != nil || encoded.Instructions != "Policy" {
		t.Fatal(encoded.Instructions, err)
	}
	req.Instructions = " \n\t"
	req.ProviderOptions = map[string]any{
		"openai": ProviderOptions{
			TrimInstructions:        true,
			PromptCacheMode:         PromptCacheModeExplicit,
			PromptCacheInstructions: true,
		},
	}
	if _, _, err = encodeRequest(
		"example-model",
		req,
		false,
	); err == nil ||
		!strings.Contains(err.Error(), "non-empty Instructions") {
		t.Fatal(err)
	}
}
