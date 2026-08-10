package kie

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestImageOptionsStrictJSONFallback(t *testing.T) {
	options, err := extractOptions(llm.GenerateImageRequest{
		ProviderOptions: map[string]any{
			"kie": map[string]any{
				"resolution":   "2K",
				"outputFormat": "png",
				"callBackUrl":  "https://example.com/callback",
				"extra":        map[string]any{"seed": float64(7)},
			},
		},
	})
	if err != nil {
		t.Fatalf("extractOptions() error = %v", err)
	}
	if options.Resolution != "2K" ||
		options.OutputFormat != "png" ||
		options.CallBackURL != "https://example.com/callback" {
		t.Fatalf("options = %#v", options)
	}
	if options.Extra["seed"] != float64(7) {
		t.Fatalf("Extra = %#v", options.Extra)
	}
}

func TestImageOptionsAcceptTypedPointer(t *testing.T) {
	options, err := extractOptions(llm.GenerateImageRequest{
		ProviderOptions: map[string]any{
			"kie": &ImageOptions{Resolution: "4K"},
		},
	})
	if err != nil || options.Resolution != "4K" {
		t.Fatalf("options = %#v, error = %v", options, err)
	}
}

func TestInvalidImageOptionsReturnBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	t.Cleanup(server.Close)

	model := newImageModel(ModelNanoBanana2, Config{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}.resolved())
	tests := []any{
		"wrong",
		map[string]any{"resolution": true},
		map[string]any{"unknown": true},
		(*ImageOptions)(nil),
	}
	for _, value := range tests {
		_, err := model.Generate(context.Background(), llm.GenerateImageRequest{
			Prompt:          "test",
			ProviderOptions: map[string]any{"kie": value},
		})
		var optionErr *llm.ProviderOptionsError
		if !errors.As(err, &optionErr) {
			t.Fatalf("value %#v error = %v, want *ProviderOptionsError", value, err)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls = %d, want 0", got)
	}
}

func TestBuildSeedreamV4TextInput(t *testing.T) {
	seed := 50331296
	nsfw := false
	got, err := buildSeedreamV4TextInput(llm.GenerateImageRequest{
		Prompt: "draw equations", N: 2, AspectRatio: "16:9", Size: "2K", Seed: &seed,
	}, ImageOptions{NSFWChecker: &nsfw})
	if err != nil {
		t.Fatalf("buildSeedreamV4TextInput() error = %v", err)
	}
	want := map[string]any{
		"prompt": "draw equations", "image_size": "landscape_16_9",
		"image_resolution": "2K", "max_images": 2, "seed": seed,
		"nsfw_checker": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
}

func TestBuildSeedreamV4EditInput(t *testing.T) {
	got, err := buildSeedreamV4EditInput(llm.GenerateImageRequest{
		Prompt: "apply this logo",
		Images: []llm.ImageInput{{URL: "https://cdn.example.com/logo.png"}},
	}, ImageOptions{ImageSize: "square_hd", Resolution: "1K", MaxImages: 1})
	if err != nil {
		t.Fatalf("buildSeedreamV4EditInput() error = %v", err)
	}
	want := map[string]any{
		"prompt":     "apply this logo",
		"image_urls": []string{"https://cdn.example.com/logo.png"},
		"image_size": "square_hd", "image_resolution": "1K", "max_images": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
}

func TestSeedreamV4EditRejectsInlineImage(t *testing.T) {
	_, err := buildSeedreamV4EditInput(llm.GenerateImageRequest{
		Prompt: "edit",
		Images: []llm.ImageInput{{Data: []byte("inline"), MediaType: "image/png"}},
	}, ImageOptions{})
	if err == nil || !strings.Contains(err.Error(), "inline images not supported") {
		t.Fatalf("error = %v, want inline image rejection", err)
	}
}

func TestSeedreamV4ValidatesRequiredFieldsAndLimits(t *testing.T) {
	elevenURLs := make([]llm.ImageInput, 11)
	for index := range elevenURLs {
		elevenURLs[index].URL = fmt.Sprintf("https://cdn.example.com/%d.png", index)
	}
	tests := []struct {
		name string
		req  llm.GenerateImageRequest
		opts ImageOptions
		edit bool
		want string
	}{
		{name: "empty prompt", req: llm.GenerateImageRequest{}, want: "prompt is required"},
		{
			name: "missing edit image",
			req:  llm.GenerateImageRequest{Prompt: "edit"},
			edit: true,
			want: "requires at least one",
		},
		{
			name: "too many edit images",
			req:  llm.GenerateImageRequest{Prompt: "edit", Images: elevenURLs},
			edit: true,
			want: "at most 10",
		},
		{name: "request count too high", req: llm.GenerateImageRequest{Prompt: "draw", N: 7}, want: "between 1 and 6"},
		{
			name: "option count negative",
			req:  llm.GenerateImageRequest{Prompt: "draw"},
			opts: ImageOptions{MaxImages: -1},
			want: "between 1 and 6",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.edit {
				_, err = buildSeedreamV4EditInput(test.req, test.opts)
			} else {
				_, err = buildSeedreamV4TextInput(test.req, test.opts)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSeedreamV4ExtraCannotOverrideReservedInput(t *testing.T) {
	reserved := []string{
		"prompt", "image_urls", "image_size", "image_resolution",
		"max_images", "seed", "nsfw_checker",
	}
	for _, key := range reserved {
		t.Run(key, func(t *testing.T) {
			_, err := buildSeedreamV4TextInput(
				llm.GenerateImageRequest{Prompt: "draw"},
				ImageOptions{Extra: map[string]any{key: "override"}},
			)
			if err == nil || !strings.Contains(err.Error(), "cannot override reserved field") {
				t.Fatalf("error = %v", err)
			}
		})
	}

	got, err := buildSeedreamV4TextInput(
		llm.GenerateImageRequest{Prompt: "draw"},
		ImageOptions{Extra: map[string]any{"custom_setting": true}},
	)
	if err != nil || got["custom_setting"] != true {
		t.Fatalf("input = %#v, error = %v", got, err)
	}
}
