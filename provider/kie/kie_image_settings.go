package kie

// ImageModelID identifies a Kie.AI image-generation model.
//
// Flux Kontext, Wan, and Midjourney are deferred.
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

	// ModelSeedreamV4TextToImage uses Seedream 4.0's text-to-image contract.
	ModelSeedreamV4TextToImage ImageModelID = "bytedance/seedream-v4-text-to-image"

	// ModelSeedreamV4Edit uses Seedream 4.0's image-edit contract.
	ModelSeedreamV4Edit ImageModelID = "bytedance/seedream-v4-edit"
)

// String implements fmt.Stringer.
func (id ImageModelID) String() string { return string(id) }
