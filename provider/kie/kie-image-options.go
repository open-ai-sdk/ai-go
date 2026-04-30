package kie

import "github.com/open-ai-sdk/ai-go/ai"

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
func imageURLs(req ai.GenerateImageRequest) []string {
	if len(req.Images) == 0 {
		return nil
	}
	urls := make([]string, 0, len(req.Images))
	for _, img := range req.Images {
		if img.URL != "" {
			urls = append(urls, img.URL)
		}
	}
	return urls
}

// buildGPTImage2TextInput builds the `input` for `gpt-image-2-text-to-image`.
// Schema fields: prompt, aspect_ratio, resolution.
func buildGPTImage2TextInput(req ai.GenerateImageRequest, opts ImageOptions) map[string]any {
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
	applyExtra(in, opts.Extra)
	return in
}

// buildGPTImage2EditInput builds the `input` for `gpt-image-2-image-to-image`.
// Schema fields: prompt, input_urls, aspect_ratio, resolution.
func buildGPTImage2EditInput(req ai.GenerateImageRequest, opts ImageOptions) map[string]any {
	in := map[string]any{}
	if req.Prompt != "" {
		in["prompt"] = req.Prompt
	}
	if urls := imageURLs(req); len(urls) > 0 {
		in["input_urls"] = urls
	}
	if req.AspectRatio != "" {
		in["aspect_ratio"] = req.AspectRatio
	}
	if res := pick(opts.Resolution, req.Size); res != "" {
		in["resolution"] = res
	}
	applyExtra(in, opts.Extra)
	return in
}

// buildNanoBanana2Input builds the `input` for `nano-banana-2`.
// Schema fields: prompt, image_input, aspect_ratio, resolution, output_format.
func buildNanoBanana2Input(req ai.GenerateImageRequest, opts ImageOptions) map[string]any {
	in := map[string]any{}
	if req.Prompt != "" {
		in["prompt"] = req.Prompt
	}
	if urls := imageURLs(req); len(urls) > 0 {
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
	applyExtra(in, opts.Extra)
	return in
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
