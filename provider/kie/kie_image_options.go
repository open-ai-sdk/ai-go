package kie

import (
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/llm"
)

// ImageOptions are Kie-specific knobs for a single Generate call. None are
// required — every model has working defaults via the upstream API.
//
// Fields are mapped onto the per-model `input` envelope below; see each
// model's upstream API reference for the canonical schema.
type ImageOptions struct {
	// Resolution maps to input.resolution for GPT Image/Nano Banana and to
	// input.image_resolution for Seedream 4: "1K" | "2K" | "4K".
	Resolution string `json:"resolution"`

	// ImageSize maps to Seedream 4's input.image_size preset, for example
	// "square_hd", "portrait_4_3", or "landscape_16_9".
	ImageSize string `json:"imageSize"`

	// MaxImages maps to Seedream 4's input.max_images. When unset, req.N is
	// used when positive and the upstream default otherwise applies.
	MaxImages int `json:"maxImages"`

	// NSFWChecker maps to Seedream 4's input.nsfw_checker. A pointer preserves
	// explicit false versus an omitted option.
	NSFWChecker *bool `json:"nsfwChecker"`

	// OutputFormat maps to `input.output_format` for nano-banana-2:
	// "jpg" | "png".
	OutputFormat string `json:"outputFormat"`

	// CallBackURL is forwarded as the top-level `callBackUrl` field; when set
	// Kie will POST a completion notification (we still poll either way).
	CallBackURL string `json:"callBackUrl"`

	// Extra adds arbitrary fields to `input`. Use this for fields that have
	// not yet been promoted to a typed option (e.g. seed). Values overwrite
	// builder defaults.
	Extra map[string]any `json:"extra"`
}

// ProviderName identifies the key used in llm.GenerateImageRequest.ProviderOptions.
func (ImageOptions) ProviderName() string { return "kie" }

// extractOptions pulls ImageOptions out of req.ProviderOptions["kie"], if any.
// Typed ImageOptions are primary; map[string]any is the strict JSON fallback.
func extractOptions(req llm.GenerateImageRequest) (ImageOptions, error) {
	if req.ProviderOptions == nil {
		return ImageOptions{}, nil
	}
	raw, ok := req.ProviderOptions["kie"]
	if !ok {
		return ImageOptions{}, nil
	}
	switch typed := raw.(type) {
	case ImageOptions:
		return typed, nil
	case *ImageOptions:
		if typed == nil {
			return ImageOptions{}, llm.ProviderOptionTypeError("kie", raw)
		}
		return *typed, nil
	case map[string]any:
		var decoded ImageOptions
		if err := llm.DecodeJSONProviderOptions("kie", typed, &decoded); err != nil {
			return ImageOptions{}, err
		}
		return decoded, nil
	default:
		return ImageOptions{}, llm.ProviderOptionTypeError("kie", raw)
	}
}

// imageURLs returns the image-input URLs from req.Images. Inline data is
// not supported on Kie input — callers must upload first via
// Provider.UploadBase64 / Provider.UploadStream and pass the resulting URL.
//
// An error is returned if any image has an empty URL (indicating inline data
// that was not uploaded), referencing the image index for context.
func imageURLs(req llm.GenerateImageRequest) ([]string, error) {
	if len(req.Images) == 0 {
		return nil, nil
	}
	urls := make([]string, 0, len(req.Images))
	for i, img := range req.Images {
		if img.URL == "" {
			return nil, fmt.Errorf(
				"kie: image[%d]: inline images not supported; upload and pass URL", i,
			)
		}
		urls = append(urls, img.URL)
	}
	return urls, nil
}

// buildGPTImage2TextInput builds the `input` for `gpt-image-2-text-to-image`.
// Schema fields: prompt, aspect_ratio, resolution, n, seed.
func buildGPTImage2TextInput(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	in := map[string]any{}
	if req.Prompt != "" {
		in["prompt"] = req.Prompt
	}
	if req.AspectRatio != "" {
		in["aspect_ratio"] = req.AspectRatio
	}
	if res := pick(opts.Resolution, req.Size); res != "" {
		in["resolution"] = res
	}
	if req.N > 0 {
		in["n"] = req.N
	}
	if req.Seed != nil {
		in["seed"] = *req.Seed
	}
	applyExtra(in, opts.Extra)
	return in, nil
}

// buildGPTImage2EditInput builds the `input` for `gpt-image-2-image-to-image`.
// Schema fields: prompt, input_urls, aspect_ratio, resolution, n, seed.
func buildGPTImage2EditInput(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	in := map[string]any{}
	if req.Prompt != "" {
		in["prompt"] = req.Prompt
	}
	urls, err := imageURLs(req)
	if err != nil {
		return nil, err
	}
	if len(urls) > 0 {
		in["input_urls"] = urls
	}
	if req.AspectRatio != "" {
		in["aspect_ratio"] = req.AspectRatio
	}
	if res := pick(opts.Resolution, req.Size); res != "" {
		in["resolution"] = res
	}
	if req.N > 0 {
		in["n"] = req.N
	}
	if req.Seed != nil {
		in["seed"] = *req.Seed
	}
	applyExtra(in, opts.Extra)
	return in, nil
}

// buildNanoBanana2Input builds the `input` for `nano-banana-2`.
// Schema fields: prompt, image_input, aspect_ratio, resolution, output_format,
// n, seed.
func buildNanoBanana2Input(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	in := map[string]any{}
	if req.Prompt != "" {
		in["prompt"] = req.Prompt
	}
	urls, err := imageURLs(req)
	if err != nil {
		return nil, err
	}
	if len(urls) > 0 {
		in["image_input"] = urls
	}
	if req.AspectRatio != "" {
		in["aspect_ratio"] = req.AspectRatio
	}
	if res := pick(opts.Resolution, req.Size); res != "" {
		in["resolution"] = res
	}
	if opts.OutputFormat != "" {
		in["output_format"] = opts.OutputFormat
	}
	if req.N > 0 {
		in["n"] = req.N
	}
	if req.Seed != nil {
		in["seed"] = *req.Seed
	}
	applyExtra(in, opts.Extra)
	return in, nil
}

// buildSeedreamV4TextInput builds the Kie Market input envelope for
// `bytedance/seedream-v4-text-to-image`.
func buildSeedreamV4TextInput(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	return buildSeedreamV4Input(req, opts, false)
}

// buildSeedreamV4EditInput builds the Kie Market input envelope for
// `bytedance/seedream-v4-edit`, including image_urls.
func buildSeedreamV4EditInput(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	return buildSeedreamV4Input(req, opts, true)
}

func buildSeedreamV4Input(req llm.GenerateImageRequest, opts ImageOptions, edit bool) (map[string]any, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("kie: seedream prompt is required")
	}
	in := map[string]any{"prompt": req.Prompt}
	if edit {
		urls, err := imageURLs(req)
		if err != nil {
			return nil, err
		}
		if len(urls) == 0 {
			return nil, fmt.Errorf("kie: seedream edit requires at least one image URL")
		}
		if len(urls) > 10 {
			return nil, fmt.Errorf("kie: seedream edit supports at most 10 image URLs")
		}
		in["image_urls"] = urls
	}
	if size := pick(opts.ImageSize, seedreamImageSize(req.AspectRatio)); size != "" {
		in["image_size"] = size
	}
	if resolution := pick(opts.Resolution, req.Size); resolution != "" {
		in["image_resolution"] = resolution
	}
	maxImages := opts.MaxImages
	if maxImages == 0 {
		maxImages = req.N
	}
	if maxImages < 0 || maxImages > 6 {
		return nil, fmt.Errorf("kie: seedream max images must be between 1 and 6 when set")
	}
	if maxImages > 0 {
		in["max_images"] = maxImages
	}
	if req.Seed != nil {
		in["seed"] = *req.Seed
	}
	if opts.NSFWChecker != nil {
		in["nsfw_checker"] = *opts.NSFWChecker
	}
	applyExtra(in, opts.Extra)
	return in, nil
}

// seedreamImageSize translates provider-neutral aspect ratios to Seedream 4
// presets. Provider-specific preset names pass through unchanged.
func seedreamImageSize(aspectRatio string) string {
	switch aspectRatio {
	case "1:1":
		return "square_hd"
	case "3:4":
		return "portrait_4_3"
	case "2:3":
		return "portrait_3_2"
	case "9:16":
		return "portrait_16_9"
	case "4:3":
		return "landscape_4_3"
	case "3:2":
		return "landscape_3_2"
	case "16:9":
		return "landscape_16_9"
	case "21:9":
		return "landscape_21_9"
	default:
		return aspectRatio
	}
}

// pick returns the first non-empty string.
func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// applyExtra copies extra into dst, overwriting existing keys.
func applyExtra(dst, extra map[string]any) {
	for k, v := range extra {
		dst[k] = v
	}
}
