package kie

import (
	"fmt"

	"github.com/open-ai-sdk/ai-go/ai"
)

// ImageOptions are Kie-specific knobs for a single Generate call. None are
// required — every model has working defaults via the upstream API.
//
// Fields are mapped onto the per-model `input` envelope below; see each
// model's upstream API reference for the canonical schema.
type ImageOptions struct {
	// Resolution maps to `input.resolution`: "1K" | "2K" | "4K".
	Resolution string

	// OutputFormat maps to `input.output_format` for nano-banana-2:
	// "jpg" | "png".
	OutputFormat string

	// CallBackURL is forwarded as the top-level `callBackUrl` field; when set
	// Kie will POST a completion notification (we still poll either way).
	CallBackURL string

	// Extra adds arbitrary fields to `input`. Use this for fields that have
	// not yet been promoted to a typed option (e.g. seed). Values overwrite
	// builder defaults.
	Extra map[string]any
}

// extractOptions pulls ImageOptions out of req.ProviderOptions["kie"], if any.
// Returns the zero ImageOptions when the key is missing or the wrong type.
func extractOptions(req ai.GenerateImageRequest) ImageOptions {
	if req.ProviderOptions == nil {
		return ImageOptions{}
	}
	raw, ok := req.ProviderOptions["kie"]
	if !ok {
		return ImageOptions{}
	}
	if v, ok := raw.(ImageOptions); ok {
		return v
	}
	if v, ok := raw.(*ImageOptions); ok && v != nil {
		return *v
	}
	return ImageOptions{}
}

// imageURLs returns the image-input URLs from req.Images. Inline data is
// not supported on Kie input — callers must upload first via
// Provider.UploadBase64 / Provider.UploadStream and pass the resulting URL.
//
// An error is returned if any image has an empty URL (indicating inline data
// that was not uploaded), referencing the image index for context.
func imageURLs(req ai.GenerateImageRequest) ([]string, error) {
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
func buildGPTImage2TextInput(req ai.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
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
func buildGPTImage2EditInput(req ai.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
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
func buildNanoBanana2Input(req ai.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
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
