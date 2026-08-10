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
		{
			name: "request count too high",
			req:  llm.GenerateImageRequest{Prompt: "draw", N: 7},
			want: "between 1 and 6",
		},
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

func TestBuildSeedreamV3Input(t *testing.T) {
	guidanceScale := 2.5
	seed := 0
	got, err := buildSeedreamV3Input(llm.GenerateImageRequest{
		Prompt: "draw a campsite", AspectRatio: "1:1", Seed: &seed,
	}, ImageOptions{GuidanceScale: &guidanceScale})
	if err != nil {
		t.Fatalf("buildSeedreamV3Input() error = %v", err)
	}
	want := map[string]any{
		"prompt": "draw a campsite", "image_size": "square_hd",
		"guidance_scale": guidanceScale, "seed": seed,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
}

func TestBuildSeedreamAspectRatioInputs(t *testing.T) {
	nsfw := false
	tests := []struct {
		name string
		cfg  seedreamAspectRatioConfig
		req  llm.GenerateImageRequest
		opts ImageOptions
		want map[string]any
	}{
		{
			name: "4.5 text to image",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "16:9"},
			opts: ImageOptions{Quality: "basic", NSFWChecker: &nsfw},
			want: map[string]any{
				"prompt": "draw", "aspect_ratio": "16:9", "quality": "basic", "nsfw_checker": false,
			},
		},
		{
			name: "5 lite image to image",
			cfg:  seedreamAspectRatioConfig{edit: true},
			req: llm.GenerateImageRequest{
				Prompt:      "edit",
				AspectRatio: "1:1",
				Images:      []llm.ImageInput{{URL: "https://cdn.example.com/source.png"}},
			},
			opts: ImageOptions{Quality: "high"},
			want: map[string]any{
				"prompt":       "edit",
				"image_urls":   []string{"https://cdn.example.com/source.png"},
				"aspect_ratio": "1:1",
				"quality":      "high",
			},
		},
		{
			name: "5 pro text to image",
			cfg:  seedreamAspectRatioConfig{outputFormat: true},
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "2:3"},
			opts: ImageOptions{Quality: "high", OutputFormat: "png"},
			want: map[string]any{
				"prompt": "draw", "aspect_ratio": "2:3", "quality": "high", "output_format": "png",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildSeedreamAspectRatioInput(test.req, test.opts, test.cfg)
			if err != nil {
				t.Fatalf("buildSeedreamAspectRatioInput() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("input = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSeedreamAspectRatioInputValidation(t *testing.T) {
	tests := []struct {
		name string
		req  llm.GenerateImageRequest
		opts ImageOptions
		cfg  seedreamAspectRatioConfig
		want string
	}{
		{name: "missing prompt", want: "prompt is required"},
		{
			name: "missing aspect ratio", req: llm.GenerateImageRequest{Prompt: "draw"},
			opts: ImageOptions{Quality: "basic"}, want: "aspect ratio is required",
		},
		{
			name: "missing quality",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
			want: "quality is required",
		},
		{
			name: "missing edit image",
			req:  llm.GenerateImageRequest{Prompt: "edit"},
			cfg:  seedreamAspectRatioConfig{edit: true},
			want: "requires at least one",
		},
		{
			name: "unsupported aspect ratio",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "5:4"},
			want: "aspect ratio",
		},
		{
			name: "unsupported quality",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
			opts: ImageOptions{Quality: "ultra"},
			cfg:  seedreamAspectRatioConfig{},
			want: "quality",
		},
		{
			name: "output format unavailable for lite",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
			opts: ImageOptions{Quality: "basic", OutputFormat: "png"},
			want: "does not support output format",
		},
		{
			name: "invalid pro output format",
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
			opts: ImageOptions{Quality: "basic", OutputFormat: "webp"},
			cfg:  seedreamAspectRatioConfig{outputFormat: true},
			want: "output format",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildSeedreamAspectRatioInput(test.req, test.opts, test.cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSeedream5LiteAcceptsUltraQuality(t *testing.T) {
	model := &ImageModel{modelID: ModelSeedreamV5LiteTextToImage}
	input, err := model.buildInput(
		llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
		ImageOptions{Quality: "ultra"},
	)
	if err != nil {
		t.Fatalf("buildInput() error = %v", err)
	}
	if input["quality"] != "ultra" {
		t.Fatalf("quality = %v, want ultra", input["quality"])
	}

	model = &ImageModel{modelID: ModelSeedreamV5ProTextToImage}
	_, err = model.buildInput(
		llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
		ImageOptions{Quality: "ultra"},
	)
	if err == nil || !strings.Contains(err.Error(), "quality") {
		t.Fatalf("pro quality error = %v, want rejection", err)
	}
}

func TestBuildSeedreamLayerDecompositionInput(t *testing.T) {
	got, err := buildSeedreamLayerDecompositionInput(llm.GenerateImageRequest{
		Prompt: "separate the parrot",
		Images: []llm.ImageInput{
			{URL: "https://cdn.example.com/image.png"},
		},
		Size: "1.5K",
	}, ImageOptions{OutputFormat: "jpeg"})
	if err != nil {
		t.Fatalf("buildSeedreamLayerDecompositionInput() error = %v", err)
	}
	want := map[string]any{
		"prompt":        "separate the parrot",
		"image_url":     "https://cdn.example.com/image.png",
		"size":          "1.5K",
		"output_format": "jpeg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
}

func TestSeedreamLayerDecompositionValidation(t *testing.T) {
	tests := []struct {
		name string
		req  llm.GenerateImageRequest
		opts ImageOptions
		want string
	}{
		{name: "no image", want: "exactly one"},
		{
			name: "multiple images",
			req: llm.GenerateImageRequest{
				Images: []llm.ImageInput{
					{URL: "https://cdn.example.com/1.png"},
					{URL: "https://cdn.example.com/2.png"},
				},
			},
			want: "exactly one",
		},
		{
			name: "invalid size",
			req: llm.GenerateImageRequest{
				Images: []llm.ImageInput{{URL: "https://cdn.example.com/1.png"}},
			},
			opts: ImageOptions{LayerSize: "4K"},
			want: "layer size",
		},
		{
			name: "invalid output",
			req: llm.GenerateImageRequest{
				Images: []llm.ImageInput{{URL: "https://cdn.example.com/1.png"}},
			},
			opts: ImageOptions{OutputFormat: "webp"},
			want: "output format",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildSeedreamLayerDecompositionInput(test.req, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestImageModelBuildInputSupportsSeedream5LiteOutputFormat(t *testing.T) {
	tests := []struct {
		name string
		id   ImageModelID
		req  llm.GenerateImageRequest
	}{
		{
			name: "text to image",
			id:   ModelSeedreamV5LiteTextToImage,
			req:  llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
		},
		{
			name: "image to image",
			id:   ModelSeedreamV5LiteImageToImage,
			req: llm.GenerateImageRequest{
				Prompt:      "edit",
				AspectRatio: "1:1",
				Images:      []llm.ImageInput{{URL: "https://cdn.example.com/source.png"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (&ImageModel{modelID: test.id}).buildInput(
				test.req,
				ImageOptions{Quality: "basic", OutputFormat: "jpeg"},
			)
			if err != nil {
				t.Fatalf("buildInput() error = %v", err)
			}
			if got["output_format"] != "jpeg" {
				t.Fatalf("output_format = %v, want jpeg", got["output_format"])
			}
		})
	}
}

func TestSeedreamEditInputImageLimits(t *testing.T) {
	tests := []struct {
		name string
		id   ImageModelID
		max  int
	}{
		{name: "4.5", id: ModelSeedreamV45Edit, max: 14},
		{name: "5 lite", id: ModelSeedreamV5LiteImageToImage, max: 14},
		{name: "5 pro", id: ModelSeedreamV5ProImageToImage, max: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := func(count int) llm.GenerateImageRequest {
				images := make([]llm.ImageInput, count)
				for index := range images {
					images[index].URL = fmt.Sprintf("https://cdn.example.com/%d.png", index)
				}
				return llm.GenerateImageRequest{
					Prompt:      "edit",
					AspectRatio: "1:1",
					Images:      images,
				}
			}
			model := &ImageModel{modelID: test.id}
			opts := ImageOptions{Quality: "basic"}
			if _, err := model.buildInput(request(test.max), opts); err != nil {
				t.Fatalf("buildInput(%d) error = %v", test.max, err)
			}
			_, err := model.buildInput(request(test.max+1), opts)
			if err == nil || !strings.Contains(err.Error(), "at most") {
				t.Fatalf("buildInput(%d) error = %v, want image-limit error", test.max+1, err)
			}
		})
	}
}

func TestSeedreamExtraCannotOverrideNewFamilyReservedInput(t *testing.T) {
	tests := []struct {
		name  string
		build func(ImageOptions) error
		key   string
	}{
		{
			name: "v3", key: "guidance_scale",
			build: func(opts ImageOptions) error {
				_, err := buildSeedreamV3Input(llm.GenerateImageRequest{Prompt: "draw"}, opts)
				return err
			},
		},
		{
			name: "5 pro", key: "quality",
			build: func(opts ImageOptions) error {
				_, err := buildSeedreamAspectRatioInput(
					llm.GenerateImageRequest{Prompt: "draw", AspectRatio: "1:1"},
					opts,
					seedreamAspectRatioConfig{outputFormat: true},
				)
				return err
			},
		},
		{
			name: "layer", key: "image_url",
			build: func(opts ImageOptions) error {
				_, err := buildSeedreamLayerDecompositionInput(
					llm.GenerateImageRequest{
						Images: []llm.ImageInput{{URL: "https://cdn.example.com/1.png"}},
					},
					opts,
				)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.build(
				ImageOptions{Quality: "basic", Extra: map[string]any{test.key: "override"}},
			)
			if err == nil || !strings.Contains(err.Error(), "cannot override reserved field") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
