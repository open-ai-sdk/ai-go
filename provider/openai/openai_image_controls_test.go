package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestImageMultipartPreservesExplicitFilenameAndDefaultName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		files := r.MultipartForm.File["image[]"]
		if len(files) != 2 {
			t.Errorf("files = %#v", files)
			return
		}
		if files[0].Filename != `original "draft".png` || files[1].Filename != "image[]-2.webp" {
			t.Errorf("filenames = %q, %q", files[0].Filename, files[1].Filename)
		}
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"AQID"}]}`)
	}))
	defer server.Close()
	_, err := NewImageModel(
		"gpt-image-1",
		Config{BaseURL: server.URL},
	).Generate(context.Background(), llm.GenerateImageRequest{
		Prompt: "restyle",
		Images: []llm.ImageInput{
			{Data: []byte{1}, MediaType: "image/png", FileName: `original "draft".png`},
			{Data: []byte{2}, MediaType: "image/webp"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestImageOptionalFirstAvailableResultAndResponseFormatOmission(t *testing.T) {
	raw := `{"data":[{}, {"b64_json":"AQID"}, {"b64_json":"?"}],"usage":42}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, present := body["response_format"]; present {
			t.Errorf("body = %#v", body)
		}
		w.Header().Set("x-request-id", "req-image")
		_, _ = io.WriteString(w, raw)
	}))
	defer server.Close()
	empty := ""
	result, err := NewImageModel(
		"custom-image",
		Config{BaseURL: server.URL},
	).Generate(context.Background(), llm.GenerateImageRequest{
		Prompt:          "render",
		ProviderOptions: map[string]any{"openai": ImageOptions{FirstImageOnly: true, ResponseFormat: &empty}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || string(result.Images[0].Data) != "\x01\x02\x03" || string(result.Raw) != raw {
		t.Fatalf("result = %#v", result)
	}
	if result.ResponseHeaders["X-Request-Id"][0] != "req-image" {
		t.Fatal(result.ResponseHeaders)
	}
	if _, err := parseImageResponse([]byte(raw), ""); err == nil || !strings.Contains(err.Error(), "missing b64_json") {
		t.Fatalf("strict default error = %v", err)
	}
}

func TestImageFirstOnlyDistinguishesEmptyBase64FromMissing(t *testing.T) {
	result, err := parseImageResponseWithOptions([]byte(`{"data":[{}, {"b64_json":""}]}`), "png", true)
	if err != nil || len(result.Images) != 1 || len(result.Images[0].Data) != 0 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}
