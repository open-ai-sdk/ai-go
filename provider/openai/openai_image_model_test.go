package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func TestImageModelGenerateUsesJSONAndPreservesRawResponse(t *testing.T) {
	t.Parallel()

	responseBody := "{\n\"created\":1,\"data\":[{\"b64_json\":\"aGVs bG8\"}]," +
		"\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5,\"input_tokens_details\":{\"image_tokens\":1}}}"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/images/generations" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "gpt-image-2" || body["prompt"] != "draw a fox" || body["n"] != float64(2) {
			t.Errorf("body = %#v", body)
		}
		if _, exists := body["response_format"]; exists {
			t.Errorf("GPT image request sent response_format: %#v", body)
		}
		if body["output_format"] != "webp" || body["output_compression"] != float64(80) {
			t.Errorf("image options = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, responseBody)
	}))
	defer server.Close()

	compression := 80
	model := NewImageModel("gpt-image-2", Config{APIKey: "secret", BaseURL: server.URL})
	result, err := model.Generate(context.Background(), llm.GenerateImageRequest{
		Prompt: "draw a fox", N: 2, Size: "1024x1024",
		ProviderOptions: map[string]any{"openai": ImageOptions{
			Quality: "high", OutputFormat: "webp", OutputCompression: &compression,
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.Images) != 1 || string(result.Images[0].Data) != "hello" ||
		result.Images[0].MediaType != "image/webp" {
		t.Fatalf("images = %#v", result.Images)
	}
	if string(result.Raw) != responseBody {
		t.Fatalf("Raw = %q, want exact %q", result.Raw, responseBody)
	}
	if result.Usage == nil || result.Usage.InputTokens != 2 || result.Usage.OutputTokens != 3 ||
		result.Usage.TotalTokens != 5 {
		t.Fatalf("Usage = %#v", result.Usage)
	}
	if result.Usage.Raw["input_tokens_details"] == nil {
		t.Fatalf("raw usage = %#v", result.Usage.Raw)
	}
}

func TestImageModelGenerationSetsResponseFormatForDALLE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body["response_format"] != "b64_json" {
			t.Errorf("response_format = %#v", body["response_format"])
		}
		_, _ = io.WriteString(writer, `{"data":[{"b64_json":"eA=="}]}`)
	}))
	defer server.Close()

	_, err := NewImageModel("dall-e-3", Config{BaseURL: server.URL}).Generate(
		context.Background(), llm.GenerateImageRequest{Prompt: "x"},
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

func TestImageModelEditUsesRepeatedMultipartImagesAndMask(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/images/edits" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := request.MultipartForm.Value["prompt"]; len(got) != 1 || got[0] != "restyle" {
			t.Errorf("prompt = %#v", got)
		}
		if _, exists := request.MultipartForm.Value["response_format"]; exists {
			t.Errorf("GPT image edit sent response_format")
		}
		images := request.MultipartForm.File["image[]"]
		if len(images) != 2 {
			t.Fatalf("image parts = %d", len(images))
		}
		for index, want := range []string{"first", "second"} {
			file, err := images[index].Open()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(file)
			_ = file.Close()
			if string(data) != want {
				t.Errorf("image[%d] = %q", index, data)
			}
		}
		masks := request.MultipartForm.File["mask"]
		if len(masks) != 1 {
			t.Fatalf("mask parts = %d", len(masks))
		}
		_, _ = io.WriteString(writer, `{"data":[{"b64_json":"b2s="}]}`)
	}))
	defer server.Close()

	model := NewImageModel("gpt-image-2", Config{BaseURL: server.URL})
	result, err := model.Generate(context.Background(), llm.GenerateImageRequest{
		Prompt: "restyle",
		Images: []llm.ImageInput{
			{Data: []byte("first"), MediaType: "image/png"},
			{Data: []byte("second"), MediaType: "image/webp"},
		},
		ProviderOptions: map[string]any{"openai": ImageOptions{
			Mask: &llm.ImageInput{Data: []byte("mask"), MediaType: "image/png"},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(result.Images[0].Data) != "ok" {
		t.Fatalf("image = %q", result.Images[0].Data)
	}
}

func TestImageModelUsesResponseOutputFormatWhenRequestOmitsIt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"output_format":"jpeg","data":[{"b64_json":"eA=="}]}`)
	}))
	defer server.Close()

	result, err := NewImageModel("gpt-image-2", Config{BaseURL: server.URL}).Generate(
		context.Background(), llm.GenerateImageRequest{Prompt: "x"},
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := result.Images[0].MediaType; got != "image/jpeg" {
		t.Fatalf("MediaType = %q, want image/jpeg", got)
	}
}

func TestImageModelRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"data":[{"b64_json":"eA=="}]}`)
	}))
	defer server.Close()

	_, err := NewImageModel("gpt-image-2", Config{
		BaseURL:               server.URL,
		ImageResponseMaxBytes: 8,
	}).Generate(context.Background(), llm.GenerateImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "response exceeds maximum size") {
		t.Fatalf("error = %v", err)
	}
}

func TestImageModelRejectsUnsafeAndInvalidInputsBeforeHTTP(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	gptImageModel := NewImageModel("gpt-image-2", Config{BaseURL: server.URL})
	sharedImageModel := NewImageModel("dall-e-3", Config{BaseURL: server.URL})
	compression := 50
	tests := []struct {
		name  string
		model *ImageModel
		req   llm.GenerateImageRequest
		want  string
	}{
		{
			name:  "url source",
			model: gptImageModel,
			req: llm.GenerateImageRequest{
				Prompt: "x",
				Images: []llm.ImageInput{{URL: "http://169.254.169.254/latest"}},
			},
			want: "URL-only",
		},
		{
			name:  "url mask",
			model: gptImageModel,
			req: llm.GenerateImageRequest{
				Prompt: "x",
				Images: []llm.ImageInput{{Data: []byte("x")}},
				ProviderOptions: map[string]any{
					"openai": ImageOptions{Mask: &llm.ImageInput{URL: "https://example.com/mask"}},
				},
			},
			want: "URL-only",
		},
		{
			name:  "too many",
			model: gptImageModel,
			req:   llm.GenerateImageRequest{Prompt: "x", Images: make([]llm.ImageInput, 17)},
			want:  "at most 16",
		},
		{
			name:  "transparent jpeg",
			model: sharedImageModel,
			req: llm.GenerateImageRequest{
				Prompt: "x",
				ProviderOptions: map[string]any{
					"openai": ImageOptions{Background: "transparent", OutputFormat: "jpeg"},
				},
			},
			want: "transparent background",
		},
		{
			name:  "gpt-image-2 transparent png",
			model: gptImageModel,
			req: llm.GenerateImageRequest{
				Prompt:          "x",
				ProviderOptions: map[string]any{"openai": ImageOptions{Background: "transparent", OutputFormat: "png"}},
			},
			want: "does not support transparent",
		},
		{
			name:  "gpt-image-2 input fidelity",
			model: gptImageModel,
			req: llm.GenerateImageRequest{
				Prompt:          "x",
				Images:          []llm.ImageInput{{Data: []byte("x")}},
				ProviderOptions: map[string]any{"openai": ImageOptions{InputFidelity: "high"}},
			},
			want: "omit input fidelity",
		},
		{
			name:  "compression png",
			model: gptImageModel,
			req: llm.GenerateImageRequest{
				Prompt: "x",
				ProviderOptions: map[string]any{
					"openai": ImageOptions{OutputFormat: "png", OutputCompression: &compression},
				},
			},
			want: "compression requires",
		},
		{
			name:  "n out of range",
			model: gptImageModel,
			req:   llm.GenerateImageRequest{Prompt: "x", N: 11},
			want:  "between 1 and 10",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.model.Generate(context.Background(), test.req)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d", calls)
	}
}

func TestImageOptionsStrictMapAndAPIErrorRedaction(t *testing.T) {
	t.Parallel()

	model := NewImageModel("gpt-image-2", Config{})
	_, err := model.Generate(context.Background(), llm.GenerateImageRequest{
		Prompt: "x", ProviderOptions: map[string]any{"openai": map[string]any{"unknown": true}},
	})
	var optionsErr *llm.ProviderOptionsError
	if !errors.As(err, &optionsErr) {
		t.Fatalf("strict options error = %T %v", err, err)
	}

	const secret = "do-not-leak-raw-debug"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"message":"invalid image","code":"bad_image"},"debug":"`+secret+`"}`)
	}))
	defer server.Close()
	_, err = NewImageModel("gpt-image-2", Config{BaseURL: server.URL}).Generate(
		context.Background(), llm.GenerateImageRequest{Prompt: "x"},
	)
	var apiErr *aikit.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	if apiErr.Code != "bad_image" || strings.Contains(err.Error(), secret) {
		t.Fatalf("API error = %v", err)
	}
}

func TestOpenAIProviderAndClientImageModel(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	provider := NewFromEnv()
	if provider.Name() != "openai" || provider.config.APIKey != "env-key" {
		t.Fatalf("provider = %#v", provider)
	}
	if provider.LanguageModel("gpt-5").ModelID() != "gpt-5" ||
		provider.ImageModel("gpt-image-2").ModelID() != "gpt-image-2" {
		t.Fatal("provider model handles have unexpected IDs")
	}

	client, err := NewClient(Config{APIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	image := client.ImageModel("gpt-image-2")
	if image.client != client {
		t.Fatal("image handle does not share owning client")
	}

	// Verify NewFromEnv copied the value rather than retaining live env state.
	t.Setenv("OPENAI_API_KEY", "changed")
	if provider.config.APIKey != "env-key" {
		t.Fatal("provider config changed with environment")
	}
}
