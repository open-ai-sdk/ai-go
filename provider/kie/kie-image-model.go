package kie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/internal/safego"
	"github.com/open-ai-sdk/ai-go/llm"
)

// ImageModel implements ai.ImageModel by submitting a Kie task and polling
// the unified job-status endpoint until terminal.
type ImageModel struct {
	modelID ImageModelID
	cfg     Config
}

var _ llm.ImageModel = (*ImageModel)(nil)

func newImageModel(modelID ImageModelID, cfg Config) *ImageModel {
	return &ImageModel{modelID: modelID, cfg: cfg}
}

// ModelID returns the Kie model identifier.
func (m *ImageModel) ModelID() string { return m.modelID.String() }

// Generate submits a createTask, polls recordInfo until terminal, then
// downloads the result images in parallel.
func (m *ImageModel) Generate(ctx context.Context, req llm.GenerateImageRequest) (*llm.GenerateImageResult, error) {
	taskID, err := m.submitTask(ctx, req)
	if err != nil {
		return nil, err
	}

	statusFn := func(ctx context.Context) (recordInfoData, bool, error) {
		info, err := m.fetchStatus(ctx, taskID)
		if err != nil {
			return recordInfoData{}, false, err
		}
		switch normalizeState(info.State) {
		case stateSuccess:
			return info, true, nil
		case stateFail:
			msg := info.FailMsg
			if msg == "" {
				msg = info.State
			}
			return recordInfoData{}, false, fmt.Errorf("kie: task %s failed: %s", taskID, msg)
		case statePending:
			return recordInfoData{}, false, nil
		default:
			// Unknown state: treat as terminal failure to avoid forever-loops.
			return recordInfoData{}, false, fmt.Errorf("kie: task %s unknown state %q", taskID, info.State)
		}
	}

	final, err := poll(ctx, poller{Interval: m.cfg.PollInterval, MaxWait: m.cfg.PollTimeout}, statusFn)
	if err != nil {
		return nil, err
	}

	urls := final.URLs()
	if len(urls) == 0 {
		return nil, fmt.Errorf("kie: task %s succeeded with no result URLs", taskID)
	}

	images, err := m.downloadImages(ctx, urls)
	if err != nil {
		return nil, err
	}
	return &ai.GenerateImageResult{Images: images}, nil
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

	body, err := json.Marshal(createTaskRequest{
		Model:       m.modelID.String(),
		Input:       input,
		CallBackURL: opts.CallBackURL,
	})
	if err != nil {
		return "", fmt.Errorf("kie: marshal createTask: %w", err)
	}

	url := buildKieURL(&m.cfg, "/api/v1/jobs/createTask")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("kie: build createTask request: %w", err)
	}
	m.applyHeaders(httpReq, "application/json")

	respBody, status, err := doHTTP(m.cfg.HTTPClient, httpReq)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", parseKieError(status, respBody)
	}

	var parsed createTaskResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("kie: parse createTask response: %w", err)
	}
	if parsed.Code != 200 || parsed.Data.TaskID == "" {
		return "", &KieError{Code: parsed.Code, Msg: parsed.Msg, Status: status, RawBody: string(respBody)}
	}
	return parsed.Data.TaskID, nil
}

// fetchStatus calls `GET /api/v1/jobs/recordInfo?taskId=...`.
//
// NOTE: The exact path is the assumed Kie convention (see kie-types.go for
// rationale). If a future doc lookup contradicts this, swap the URL here in
// one place — the rest of the SDK is endpoint-agnostic.
func (m *ImageModel) fetchStatus(ctx context.Context, taskID string) (recordInfoData, error) {
	url := buildKieURL(&m.cfg, "/api/v1/jobs/recordInfo") + "?taskId=" + taskID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return recordInfoData{}, fmt.Errorf("kie: build recordInfo request: %w", err)
	}
	m.applyHeaders(httpReq, "")

	respBody, status, err := doHTTP(m.cfg.HTTPClient, httpReq)
	if err != nil {
		return recordInfoData{}, err
	}
	if status != http.StatusOK {
		return recordInfoData{}, parseKieError(status, respBody)
	}

	var parsed recordInfoResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return recordInfoData{}, fmt.Errorf("kie: parse recordInfo response: %w", err)
	}
	if parsed.Code != 200 {
		return recordInfoData{}, &KieError{
			Code: parsed.Code, Msg: parsed.Msg,
			Status: status, RawBody: string(respBody),
		}
	}
	return parsed.Data, nil
}

// buildInput dispatches to the per-model option builder.
func (m *ImageModel) buildInput(req llm.GenerateImageRequest, opts ImageOptions) (map[string]any, error) {
	switch m.modelID {
	case ModelGPTImage2TextToImage:
		return buildGPTImage2TextInput(req, opts)
	case ModelGPTImage2ImageToImage:
		return buildGPTImage2EditInput(req, opts)
	case ModelNanoBanana2:
		return buildNanoBanana2Input(req, opts)
	default:
		return nil, fmt.Errorf("kie: unsupported model %q", m.modelID)
	}
}

// downloadImages fetches each URL in parallel. HTTPS-only — non-https URLs are
// rejected to avoid accidental plaintext fetches.
func (m *ImageModel) downloadImages(ctx context.Context, urls []string) ([]ai.GeneratedImage, error) {
	results := make([]ai.GeneratedImage, len(urls))
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
			defer safego.Recover(nil, func(err error) { errs[idx] = err })
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

func (m *ImageModel) downloadOne(ctx context.Context, url string) (ai.GeneratedImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ai.GeneratedImage{}, fmt.Errorf("kie: build download request: %w", err)
	}
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return ai.GeneratedImage{}, fmt.Errorf("kie: download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ai.GeneratedImage{}, fmt.Errorf("kie: download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ai.GeneratedImage{}, fmt.Errorf("kie: read image body: %w", err)
	}
	return ai.GeneratedImage{
		Data:      data,
		MediaType: resp.Header.Get("Content-Type"),
	}, nil
}

// applyHeaders sets Auth + content-type + caller-supplied extra headers.
func (m *ImageModel) applyHeaders(req *http.Request, contentType string) {
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if m.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	}
	for k, v := range m.cfg.Headers {
		req.Header.Set(k, v)
	}
}

// taskState is the canonical (lowercased) state enum.
type taskState int

const (
	stateUnknown taskState = iota
	statePending
	stateSuccess
	stateFail
)

func normalizeState(s string) taskState {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "succeed", "succeeded":
		return stateSuccess
	case "fail", "failed", "error":
		return stateFail
	case "waiting", "queuing", "queued", "processing", "running", "generating":
		return statePending
	default:
		return stateUnknown
	}
}

// doHTTP runs req, returning body + status. Errors at the transport layer are
// returned verbatim so callers (and tests) can match on them.
func doHTTP(client *http.Client, req *http.Request) ([]byte, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		// Surface ctx errors directly so callers can errors.Is(ctx.Err()).
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		return nil, 0, fmt.Errorf("kie: http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("kie: read body: %w", err)
	}
	return body, resp.StatusCode, nil
}
