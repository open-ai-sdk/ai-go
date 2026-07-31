package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

type weatherInput struct {
	Location string  `json:"location"          description:"City and state"`
	Unit     string  `json:"unit,omitempty"    description:"Temperature unit"              enum:"celsius,fahrenheit"`
	MaxTemp  float64 `json:"maxTemp,omitempty" description:"Maximum temperature threshold"`
	Verbose  bool    `json:"verbose"           description:"Include extra details"`
}

type optionalInput struct {
	Required string  `json:"required"`
	Optional *string `json:"optional,omitempty"`
}

type skippedInput struct {
	Visible string `json:"visible"`
	Hidden  string `json:"-"`
}

func newWeatherTool(t *testing.T) *tool.Tool {
	t.Helper()
	result, err := tool.New(
		"get_weather",
		"Get the current weather",
		func(_ context.Context, _ weatherInput) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestNewDerivesRequiredFields(t *testing.T) {
	definition := newWeatherTool(t).Describe()
	if definition.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", definition.Name)
	}
	if definition.Description != "Get the current weather" {
		t.Errorf("Description = %q", definition.Description)
	}
	if definition.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v, want object", definition.InputSchema["type"])
	}

	properties, ok := definition.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	required, _ := definition.InputSchema["required"].([]string)
	requiredSet := make(map[string]bool, len(required))
	for _, name := range required {
		requiredSet[name] = true
	}
	if !requiredSet["location"] || !requiredSet["verbose"] {
		t.Errorf("required = %v, want location and verbose", required)
	}
	if _, exists := properties["location"]; !exists {
		t.Error("schema missing location")
	}
}

func TestNewTreatsPointerFieldsAsOptional(t *testing.T) {
	result, err := tool.New(
		"optional",
		"Test optional fields",
		func(_ context.Context, _ optionalInput) (struct{}, error) {
			return struct{}{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	required, _ := result.Describe().InputSchema["required"].([]string)
	requiredSet := make(map[string]bool, len(required))
	for _, name := range required {
		requiredSet[name] = true
	}
	if !requiredSet["required"] || requiredSet["optional"] {
		t.Errorf("required = %v", required)
	}
}

func TestNewHonorsJSONDescriptionAndEnumTags(t *testing.T) {
	definition := newWeatherTool(t).Describe()
	properties := definition.InputSchema["properties"].(map[string]any)
	unit := properties["unit"].(map[string]any)
	if values, ok := unit["enum"].([]any); !ok || len(values) != 2 {
		t.Errorf("enum = %#v, want two values", unit["enum"])
	}
	location := properties["location"].(map[string]any)
	if location["description"] != "City and state" {
		t.Errorf("description = %v", location["description"])
	}

	skipped, err := tool.New(
		"skip",
		"Test skipped fields",
		func(_ context.Context, _ skippedInput) (bool, error) { return true, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	skippedProperties := skipped.Describe().InputSchema["properties"].(map[string]any)
	if _, exists := skippedProperties["Hidden"]; exists {
		t.Error("json:\"-\" field should be excluded")
	}
}

func TestNewInvokeUnmarshalsInputAndMarshalsOutput(t *testing.T) {
	var captured weatherInput
	result, err := tool.New(
		"get_weather",
		"Get weather",
		func(_ context.Context, input weatherInput) (map[string]any, error) {
			captured = input
			return map[string]any{"summary": "sunny", "temperature": 72}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	input, _ := json.Marshal(weatherInput{
		Location: "San Francisco, CA",
		Unit:     "fahrenheit",
	})
	output, err := result.Invoke(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Location != "San Francisco, CA" {
		t.Errorf("Location = %q", captured.Location)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("output is not JSON: %s: %v", output, err)
	}
	if decoded["summary"] != "sunny" {
		t.Errorf("output = %s", output)
	}
}

func TestNewInvokeClassifiesInputAndExecutionErrors(t *testing.T) {
	handlerErr := errors.New("upstream failed")
	result, err := tool.New(
		"get_weather",
		"Get weather",
		func(_ context.Context, _ weatherInput) (string, error) {
			return "", handlerErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := result.Invoke(context.Background(), json.RawMessage("not-json")); !errors.Is(err, tool.ErrInput) {
		t.Fatalf("invalid JSON error = %v, want ErrInput", err)
	}
	if _, err := result.Invoke(context.Background(), json.RawMessage(`{}`)); !errors.Is(err, tool.ErrExecution) {
		t.Fatalf("handler error = %v, want ErrExecution", err)
	}
}

func TestNewPreservesWrappedTypedSentinel(t *testing.T) {
	result, err := tool.New(
		"guarded",
		"Guarded operation",
		func(_ context.Context, _ struct{}) (string, error) {
			return "", errors.Join(errors.New("policy"), tool.ErrDenied)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = result.Invoke(context.Background(), json.RawMessage(`{}`))
	if !errors.Is(err, tool.ErrDenied) {
		t.Fatalf("error = %v, want ErrDenied", err)
	}
	if errors.Is(err, tool.ErrExecution) {
		t.Fatalf("error = %v, must not also be classified as execution", err)
	}
}
