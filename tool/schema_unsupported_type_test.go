package tool_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/tool"
)

type customDecoded struct {
	Value string
}

func (*customDecoded) UnmarshalJSON([]byte) error { return nil }

func TestNewRejectsUnsupportedInputAtConstruction(t *testing.T) {
	type input struct {
		Callback func() `json:"callback"`
	}

	_, err := tool.New(
		"unsupported",
		"Unsupported input",
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err == nil {
		t.Fatal("expected construction error")
	}
	if !strings.Contains(err.Error(), "callback") {
		t.Fatalf("error %q does not name field callback", err)
	}
}

func TestNewModelsTimeFieldAsDateTimeString(t *testing.T) {
	type input struct {
		At time.Time `json:"at"`
	}
	result, err := tool.New(
		"scheduled",
		"Scheduled operation",
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	properties := result.Describe().InputSchema["properties"].(map[string]any)
	at := properties["at"].(map[string]any)
	if at["type"] != "string" || at["format"] != "date-time" {
		t.Fatalf("at schema = %#v", at)
	}
}

func TestNewRejectsPromotedAnonymousStruct(t *testing.T) {
	type Embedded struct {
		Value string `json:"value"`
	}
	type input struct {
		Embedded
	}
	_, err := tool.New(
		"embedded",
		"Embedded input",
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err == nil {
		t.Fatal("expected anonymous embedded struct error")
	}
	if !strings.Contains(err.Error(), "embedded") {
		t.Fatalf("error %q does not name embedded field", err)
	}
}

func TestNewRejectsCustomJSONDecodedField(t *testing.T) {
	type input struct {
		Custom customDecoded `json:"custom"`
	}
	_, err := tool.New(
		"custom",
		"Custom input",
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err == nil {
		t.Fatal("expected custom JSON decoding error")
	}
	if !strings.Contains(err.Error(), "custom") {
		t.Fatalf("error %q does not name custom field", err)
	}
}
