package kie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"unicode/utf8"
)

// JobsReply preserves the jobs envelope without imposing application-specific
// acceptance rules. ReadError is separate because a received HTTP status can
// determine whether a non-idempotent submission was rejected or is ambiguous.
type JobsReply struct {
	Body       []byte
	StatusCode int
	ReadError  error
}

// CreateTaskReply submits exactly once and returns the untranslated envelope.
// It does not retry an ambiguous submission or interpret provider codes.
func (p *Provider) CreateTaskReply(
	ctx context.Context,
	model string,
	input map[string]any,
	opts ...TaskOption,
) (JobsReply, error) {
	return p.jobs.createTaskReply(ctx, model, input, opts...)
}

func (j jobsClient) createTaskReply(
	ctx context.Context,
	model string,
	input map[string]any,
	opts ...TaskOption,
) (JobsReply, error) {
	payload := createTaskRequest{Model: model, Input: input}
	for _, opt := range opts {
		opt(&payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return JobsReply{}, fmt.Errorf("kie: marshal createTask: %w", err)
	}
	return j.reply(ctx, http.MethodPost, buildKieURL(&j.cfg, "/api/v1/jobs/createTask"), body)
}

// TaskStatusReply fetches a task once without normalizing states, numeric
// failure codes, credits, or resultJson.
func (p *Provider) TaskStatusReply(ctx context.Context, taskID string) (JobsReply, error) {
	return p.jobs.taskStatusReply(ctx, taskID)
}

func (j jobsClient) taskStatusReply(ctx context.Context, taskID string) (JobsReply, error) {
	return j.reply(
		ctx,
		http.MethodGet,
		buildKieURL(&j.cfg, "/api/v1/jobs/recordInfo")+"?taskId="+url.QueryEscape(taskID),
		nil,
	)
}

func (j jobsClient) reply(ctx context.Context, method, endpoint string, body []byte) (JobsReply, error) {
	if j.clientErr != nil {
		return JobsReply{}, j.clientErr
	}
	req, err := j.client.NewRequest(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return JobsReply{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return JobsReply{}, err
	}
	defer resp.Body.Close()
	reply := JobsReply{StatusCode: resp.StatusCode}
	cap := j.cfg.MaxReplyBytes
	if cap > 0 && resp.ContentLength > cap {
		reply.ReadError = errors.New("reply body too large")
		return reply, nil
	}
	var reader io.Reader = resp.Body
	if cap > 0 {
		reader = io.LimitReader(reader, cap+1)
	}
	reply.Body, err = io.ReadAll(reader)
	switch {
	case err != nil:
		reply.ReadError = fmt.Errorf("reply body: %w", err)
	case cap > 0 && int64(len(reply.Body)) > cap:
		reply.ReadError = errors.New("reply body too large")
	case j.cfg.RequireUTF8Reply && !utf8.Valid(reply.Body):
		reply.ReadError = errors.New("reply body is not UTF-8")
	}
	return reply, nil
}

// SubmitVideoReply sends the explicit input envelope unchanged using this model's
// ID. The raw reply lets applications apply their durable acceptance policy.
func (m *VideoModel) SubmitVideoReply(
	ctx context.Context,
	reqInput map[string]any,
	opts ...TaskOption,
) (JobsReply, error) {
	return m.jobs.createTaskReply(ctx, m.modelID, reqInput, opts...)
}

// VideoStatusReply fetches the untranslated jobs envelope once.
func (m *VideoModel) VideoStatusReply(ctx context.Context, taskID string) (JobsReply, error) {
	return m.jobs.taskStatusReply(ctx, taskID)
}
