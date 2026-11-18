package kie

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/open-ai-sdk/ai-go/llm"
)

// VideoOptions are Kie-specific knobs for a single video call, read from
// llm.GenerateVideoRequest.ProviderOptions["kie"].
type VideoOptions struct {
	// CallBackURL is forwarded as the top-level `callBackUrl` field.
	CallBackURL string `json:"callBackUrl"`

	// RawInput is the base `input` envelope. The request fields are added as
	// `prompt`, `image_urls`, `duration` (whole seconds, as a string) and
	// `aspect_ratio` unless RawInput already sets the key.
	RawInput map[string]any `json:"rawInput"`
}

// ProviderName identifies the key used in llm.GenerateVideoRequest.ProviderOptions.
func (VideoOptions) ProviderName() string { return "kie" }

func extractVideoOptions(req llm.GenerateVideoRequest) (VideoOptions, error) {
	raw, ok := req.ProviderOptions["kie"]
	if !ok {
		return VideoOptions{}, nil
	}
	switch typed := raw.(type) {
	case VideoOptions:
		return typed, nil
	case *VideoOptions:
		if typed == nil {
			return VideoOptions{}, llm.ProviderOptionTypeError("kie", raw)
		}
		return *typed, nil
	case map[string]any:
		var decoded VideoOptions
		if err := llm.DecodeJSONProviderOptions("kie", typed, &decoded); err != nil {
			return VideoOptions{}, err
		}
		return decoded, nil
	default:
		return VideoOptions{}, llm.ProviderOptionTypeError("kie", raw)
	}
}

// VideoModel implements llm.VideoModel over the Kie jobs API. Any model id is
// accepted. Besides the blocking GenerateVideo it exposes a durable split:
// SubmitVideo returns a task id that VideoStatus can poll later, even from
// another process.
type VideoModel struct {
	modelID string
	cfg     Config
	jobs    jobsClient
}

var _ llm.VideoModel = (*VideoModel)(nil)

func newVideoModel(modelID string, cfg Config) *VideoModel {
	cfg = cfg.resolved()
	return &VideoModel{modelID: modelID, cfg: cfg, jobs: newJobsClient(cfg)}
}

// ModelID returns the Kie model identifier.
func (m *VideoModel) ModelID() string { return m.modelID }

// SubmitVideo creates the video task and returns its task id without waiting.
func (m *VideoModel) SubmitVideo(ctx context.Context, req llm.GenerateVideoRequest) (string, error) {
	opts, err := extractVideoOptions(req)
	if err != nil {
		return "", err
	}
	if req.Duration < 0 {
		return "", errors.New("kie: video duration must not be negative")
	}
	input := passthroughInput(opts.RawInput, req.Prompt, req.ImageURLs, nil)
	if _, ok := input["duration"]; !ok && req.Duration > 0 {
		input["duration"] = strconv.Itoa(int(req.Duration.Seconds()))
	}
	if _, ok := input["aspect_ratio"]; !ok && req.AspectRatio != "" {
		input["aspect_ratio"] = req.AspectRatio
	}
	var taskOpts []TaskOption
	if opts.CallBackURL != "" {
		taskOpts = append(taskOpts, WithCallBackURL(opts.CallBackURL))
	}
	return m.jobs.createTask(ctx, m.modelID, input, taskOpts...)
}

// VideoStatus fetches the current state of a previously submitted task once.
func (m *VideoModel) VideoStatus(ctx context.Context, taskID string) (TaskRecord, error) {
	return m.jobs.taskStatus(ctx, taskID)
}

// GenerateVideo submits the task and polls until it succeeds, fails, the poll
// timeout elapses, or ctx is cancelled.
func (m *VideoModel) GenerateVideo(
	ctx context.Context,
	req llm.GenerateVideoRequest,
) (*llm.GenerateVideoResult, error) {
	taskID, err := m.SubmitVideo(ctx, req)
	if err != nil {
		return nil, err
	}
	final, err := m.jobs.await(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if len(final.ResultURLs) == 0 {
		return nil, fmt.Errorf("kie: task %s succeeded with no result URLs", taskID)
	}
	return &llm.GenerateVideoResult{URLs: final.ResultURLs, Raw: final.Raw}, nil
}
