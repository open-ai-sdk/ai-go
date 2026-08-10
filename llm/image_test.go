package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateImageResultDoesNotSerializeRawResponse(t *testing.T) {
	result := GenerateImageResult{Raw: json.RawMessage(`{"provider_secret":"sensitive"}`)}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "provider_secret") {
		t.Fatalf("Marshal() leaked raw provider response: %s", encoded)
	}
	if string(result.Raw) != `{"provider_secret":"sensitive"}` {
		t.Fatalf("Raw = %s", result.Raw)
	}
}
