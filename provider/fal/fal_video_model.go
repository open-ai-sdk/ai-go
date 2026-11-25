package fal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/open-ai-sdk/ai-go/llm"
)

// VideoOptions are fal-specific knobs for a single video call, read from
// llm.GenerateVideoRequest.ProviderOptions["fal"].
type VideoOptions struct {
	// RawInput is the base model input. The request fields are added as
	// `prompt`, `image_url` (one image) or `image_urls` (several),
	// `duration` (whole seconds, as a string) and `aspect_ratio` unless
	// RawInput already sets the key (either image key suppresses both).
	RawInput map[string]any `json:"rawInput"`
}

// ProviderName identifies the key used in llm.GenerateVideoRequest.ProviderOptions.
func (VideoOptions) ProviderName() string { return "fal" }

func extractVideoOptions(req llm.GenerateVideoRequest) (VideoOptions, error) {
	raw, ok := req.ProviderOptions["fal"]
	if !ok {
		return VideoOptions{}, nil
	}
	switch typed := raw.(type) {
	case VideoOptions:
		return typed, nil
	case *VideoOptions:
		if typed == nil {
			return VideoOptions{}, llm.ProviderOptionTypeError("fal", raw)
		}
		return *typed, nil
	case map[string]any:
		var decoded VideoOptions
		if err := llm.DecodeJSONProviderOptions("fal", typed, &decoded); err != nil {
			return VideoOptions{}, err
		}
		return decoded, nil
	default:
		return VideoOptions{}, llm.ProviderOptionTypeError("fal", raw)
	}
}

// VideoModel implements llm.VideoModel over the fal queue API for one
// endpoint id. Besides the blocking GenerateVideo it exposes a durable split:
// SubmitVideo returns a request id that StatusReply/ResultReply can query
// later, even from another process.
type VideoModel struct {
	endpointID string
	cfg        Config
	queue      queueClient
}

var _ llm.VideoModel = (*VideoModel)(nil)

func newVideoModel(endpointID string, cfg Config) *VideoModel {
	cfg = cfg.resolved()
	return &VideoModel{endpointID: endpointID, cfg: cfg, queue: newQueueClient(endpointID, cfg)}
}

// ModelID returns the fal endpoint id.
func (m *VideoModel) ModelID() string { return m.endpointID }

// SubmitVideo enqueues the request and returns its request id without waiting.
// A failure is a *SubmitError when fal answered, so callers can tell a
// rejected submission from an ambiguous one.
func (m *VideoModel) SubmitVideo(ctx context.Context, req llm.GenerateVideoRequest) (string, error) {
	input, err := videoInput(req)
	if err != nil {
		return "", err
	}
	reply, err := m.SubmitReply(ctx, input)
	if err != nil {
		return "", err
	}
	accepted, err := DecodeSubmission(reply.StatusCode, reply.Body, reply.ReadError)
	if err != nil {
		return "", err
	}
	return accepted.RequestID, nil
}

func videoInput(req llm.GenerateVideoRequest) (map[string]any, error) {
	opts, err := extractVideoOptions(req)
	if err != nil {
		return nil, err
	}
	if req.Duration < 0 {
		return nil, errors.New("fal: video duration must not be negative")
	}
	input := make(map[string]any, len(opts.RawInput)+4)
	for k, v := range opts.RawInput {
		input[k] = v
	}
	setDefault := func(key string, value any) {
		if _, ok := input[key]; !ok {
			input[key] = value
		}
	}
	if req.Prompt != "" {
		setDefault("prompt", req.Prompt)
	}
	_, hasImage := input["image_url"]
	_, hasImages := input["image_urls"]
	if !hasImage && !hasImages {
		switch len(req.ImageURLs) {
		case 0:
		case 1:
			input["image_url"] = req.ImageURLs[0]
		default:
			input["image_urls"] = req.ImageURLs
		}
	}
	if req.Duration > 0 {
		setDefault("duration", strconv.Itoa(int(req.Duration.Seconds())))
	}
	if req.AspectRatio != "" {
		setDefault("aspect_ratio", req.AspectRatio)
	}
	return input, nil
}

// GenerateVideo submits the request, polls its status until it completes,
// the poll timeout elapses, or ctx is cancelled, then fetches the result.
func (m *VideoModel) GenerateVideo(
	ctx context.Context,
	req llm.GenerateVideoRequest,
) (*llm.GenerateVideoResult, error) {
	requestID, err := m.SubmitVideo(ctx, req)
	if err != nil {
		return nil, err
	}
	_, err = poll(
		ctx,
		poller{Interval: m.cfg.PollInterval, MaxWait: m.cfg.PollTimeout},
		func(ctx context.Context) (struct{}, bool, error) {
			reply, err := m.StatusReply(ctx, requestID)
			if err != nil {
				return struct{}{}, false, err
			}
			status, err := DecodeStatus(reply.StatusCode, reply.Body, reply.ReadError)
			if err != nil {
				return struct{}{}, false, fmt.Errorf("fal: request %s: %w", requestID, err)
			}
			if status.State != "completed" {
				return struct{}{}, false, nil
			}
			if status.Error != nil {
				return struct{}{}, false, fmt.Errorf("fal: request %s failed: %s", requestID, *status.Error)
			}
			return struct{}{}, true, nil
		},
	)
	if err != nil {
		return nil, err
	}
	reply, err := m.ResultReply(ctx, requestID)
	if err != nil {
		return nil, err
	}
	result, err := DecodeVideoResult(reply.StatusCode, reply.Body, reply.ReadError)
	if err != nil {
		return nil, fmt.Errorf("fal: request %s: %w", requestID, err)
	}
	if result.State != "succeeded" {
		message := "no detail"
		if result.Message != nil {
			message = *result.Message
		}
		return nil, fmt.Errorf("fal: request %s failed: %s", requestID, message)
	}
	return &llm.GenerateVideoResult{URLs: []string{result.VideoURL}, Raw: json.RawMessage(reply.Body)}, nil
}
