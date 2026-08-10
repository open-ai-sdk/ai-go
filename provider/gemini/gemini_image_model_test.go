package gemini

import (
	"bytes"
	"testing"
)

func TestImageModelParseResponsePreservesRaw(t *testing.T) {
	raw := []byte(
		`{"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5},"providerField":"preserved"}`,
	)
	result, err := (&ImageModel{}).parseResponse(raw)
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if !bytes.Equal(result.Raw, raw) {
		t.Fatalf("Raw = %s, want exact %s", result.Raw, raw)
	}
	if len(result.Images) != 1 || string(result.Images[0].Data) != "image" {
		t.Fatalf("Images = %#v", result.Images)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 5 {
		t.Fatalf("Usage = %#v", result.Usage)
	}

	raw[0] = 'x'
	if result.Raw[0] != '{' {
		t.Fatalf("Raw aliases caller buffer")
	}
}
