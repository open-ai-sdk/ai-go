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

	// ModelSeedreamV3 uses Seedream 3.0's text-to-image contract.
	ModelSeedreamV3 ImageModelID = "bytedance/seedream"

	// ModelSeedreamV45TextToImage uses Seedream 4.5's text-to-image contract.
	ModelSeedreamV45TextToImage ImageModelID = "seedream/4.5-text-to-image"

	// ModelSeedreamV45Edit uses Seedream 4.5's image-edit contract.
	ModelSeedreamV45Edit ImageModelID = "seedream/4.5-edit"

	// ModelSeedreamV5LiteTextToImage uses Seedream 5.0 Lite's text-to-image contract.
	ModelSeedreamV5LiteTextToImage ImageModelID = "seedream/5-lite-text-to-image"

	// ModelSeedreamV5LiteImageToImage uses Seedream 5.0 Lite's image-to-image contract.
	ModelSeedreamV5LiteImageToImage ImageModelID = "seedream/5-lite-image-to-image"

	// ModelSeedreamV5ProTextToImage uses Seedream 5.0 Pro's text-to-image contract.
	ModelSeedreamV5ProTextToImage ImageModelID = "seedream/5-pro-text-to-image"

	// ModelSeedreamV5ProImageToImage uses Seedream 5.0 Pro's image-to-image contract.
	ModelSeedreamV5ProImageToImage ImageModelID = "seedream/5-pro-image-to-image"

	// ModelSeedreamV5ProLayerDecomposition separates one source image into layers.
	ModelSeedreamV5ProLayerDecomposition ImageModelID = "seedream/5-pro-layer-decomposition"
)

// String implements fmt.Stringer.
func (id ImageModelID) String() string { return string(id) }
