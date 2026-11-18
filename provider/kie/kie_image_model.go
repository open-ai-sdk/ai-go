package kie

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/open-ai-sdk/ai-go/llm"
)

// ImageModel implements aikit.ImageModel by submitting a Kie task and polling
// the unified job-status endpoint until terminal.
type ImageModel struct {
	modelID ImageModelID
	cfg     Config
	jobs    jobsClient
}

var _ llm.ImageModel = (*ImageModel)(nil)

func newImageModel(modelID ImageModelID, cfg Config) *ImageModel {
	cfg = cfg.resolved()
	return &ImageModel{modelID: modelID, cfg: cfg, jobs: newJobsClient(cfg)}
}

// ModelID returns the Kie model identifier.
func (m *ImageModel) ModelID() string { return m.modelID.String() }

// Generate submits a createTask, polls recordInfo until terminal, then
// downloads the result images in parallel.
func (m *ImageModel) Generate(
	ctx context.Context,
	req llm.GenerateImageRequest,
) (*llm.GenerateImageResult, error) {
	taskID, err := m.submitTask(ctx, req)
	if err != nil {
		return nil, err
	}
	final, err := m.jobs.await(ctx, taskID)
	if err != nil {
		return nil, err
	}

	urls := final.ResultURLs
	if len(urls) == 0 {
		return nil, fmt.Errorf("kie: task %s succeeded with no result URLs", taskID)
	}

	images, err := m.downloadImages(ctx, urls)
	if err != nil {
		return nil, err
	}
	return &llm.GenerateImageResult{Images: images, Raw: final.Raw}, nil
}

// submitTask serializes the per-model `input` envelope and POSTs createTask.
func (m *ImageModel) submitTask(ctx context.Context, req llm.GenerateImageRequest) (string, error) {
	opts, err := extractOptions(req)
	if err != nil {
		return "", err
	}
	input, err := m.buildInput(req, opts)
	if err != nil {
		return "", err
	}

	var taskOpts []TaskOption
	if opts.CallBackURL != "" {
		taskOpts = append(taskOpts, WithCallBackURL(opts.CallBackURL))
	}
	return m.jobs.createTask(ctx, m.modelID.String(), input, taskOpts...)
}

// buildInput dispatches to the per-model option builder.
func (m *ImageModel) buildInput(
	req llm.GenerateImageRequest,
	opts ImageOptions,
) (map[string]any, error) {
	switch m.modelID {
	case ModelGPTImage2TextToImage:
		return buildGPTImage2TextInput(req, opts)
	case ModelGPTImage2ImageToImage:
		return buildGPTImage2EditInput(req, opts)
	case ModelNanoBanana2:
		return buildNanoBanana2Input(req, opts)
	case ModelSeedreamV4TextToImage:
		return buildSeedreamV4TextInput(req, opts)
	case ModelSeedreamV4Edit:
		return buildSeedreamV4EditInput(req, opts)
	case ModelSeedreamV3:
		return buildSeedreamV3Input(req, opts)
	case ModelSeedreamV45TextToImage:
		return buildSeedreamAspectRatioInput(req, opts, seedreamAspectRatioConfig{})
	case ModelSeedreamV45Edit:
		return buildSeedreamAspectRatioInput(
			req,
			opts,
			seedreamAspectRatioConfig{edit: true, maxInputImages: 14},
		)
	case ModelSeedreamV5LiteTextToImage:
		return buildSeedreamAspectRatioInput(
			req,
			opts,
			seedreamAspectRatioConfig{allowUltraQuality: true, outputFormat: true},
		)
	case ModelSeedreamV5LiteImageToImage:
		return buildSeedreamAspectRatioInput(
			req,
			opts,
			seedreamAspectRatioConfig{edit: true, allowUltraQuality: true, outputFormat: true, maxInputImages: 14},
		)
	case ModelSeedreamV5ProTextToImage:
		return buildSeedreamAspectRatioInput(
			req,
			opts,
			seedreamAspectRatioConfig{outputFormat: true},
		)
	case ModelSeedreamV5ProImageToImage:
		return buildSeedreamAspectRatioInput(
			req,
			opts,
			seedreamAspectRatioConfig{edit: true, outputFormat: true, maxInputImages: 10},
		)
	case ModelSeedreamV5ProLayerDecomposition:
		return buildSeedreamLayerDecompositionInput(req, opts)
	default:
		return buildPassthroughImageInput(req, opts)
	}
}

// downloadImages fetches each URL in parallel. HTTPS-only — non-https URLs are
// rejected to avoid accidental plaintext fetches.
func (m *ImageModel) downloadImages(
	ctx context.Context,
	urls []string,
) ([]llm.GeneratedImage, error) {
	results := make([]llm.GeneratedImage, len(urls))
	errs := make([]error, len(urls))

	var wg sync.WaitGroup
	for i, u := range urls {
		if !strings.HasPrefix(u, "https://") {
			errs[i] = fmt.Errorf("kie: refusing non-https url %q", u)
			continue
		}
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			// A panic in a single download surfaces as that entry's error
			// instead of crashing the process; wg.Done still runs.
			defer recoverAsError(func(err error) { errs[idx] = err })
			img, err := m.downloadOne(ctx, url)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = img
		}(i, u)
	}
	wg.Wait()

	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}
	return results, nil
}

func (m *ImageModel) downloadOne(ctx context.Context, url string) (llm.GeneratedImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return llm.GeneratedImage{}, fmt.Errorf("kie: build download request: %w", err)
	}
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return llm.GeneratedImage{}, fmt.Errorf("kie: download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return llm.GeneratedImage{}, fmt.Errorf("kie: download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.GeneratedImage{}, fmt.Errorf("kie: read image body: %w", err)
	}
	return llm.GeneratedImage{
		Data:      data,
		MediaType: resp.Header.Get("Content-Type"),
	}, nil
}
