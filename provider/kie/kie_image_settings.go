package kie

// ImageModelID identifies a Kie.AI image-generation model.
//
// v1 scope: three models. Flux Kontext, Wan, and Midjourney are deferred.
type ImageModelID string

const (
	// ModelGPTImage2TextToImage drives `POST /api/v1/jobs/createTask` with
	// `model: "gpt-image-2-text-to-image"`. Input fields:
	// {prompt, aspect_ratio, resolution}.
	ModelGPTImage2TextToImage ImageModelID = "gpt-image-2-text-to-image"

	// ModelGPTImage2ImageToImage drives the same endpoint with
	// `model: "gpt-image-2-image-to-image"`. Input fields:
	// {prompt, input_urls, aspect_ratio, resolution}.
	ModelGPTImage2ImageToImage ImageModelID = "gpt-image-2-image-to-image"

	// ModelNanoBanana2 drives the same endpoint with `model: "nano-banana-2"`.
	// Input fields: {prompt, image_input, aspect_ratio, resolution, output_format}.
	ModelNanoBanana2 ImageModelID = "nano-banana-2"
)

// String implements fmt.Stringer.
func (id ImageModelID) String() string { return string(id) }
