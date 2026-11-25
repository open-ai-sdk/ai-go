package fal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/open-ai-sdk/ai-go/transport"
)

// QueueReply preserves a queue response without imposing application-specific
// acceptance rules. ReadError is separate because a received HTTP status can
// determine whether a non-idempotent submission was rejected or is ambiguous.
type QueueReply struct {
	Body       []byte
	StatusCode int
	ReadError  error
}

// queueClient performs single, never-retried queue exchanges for one endpoint.
type queueClient struct {
	cfg       Config
	client    *transport.Client
	clientErr error
	// segments are the endpoint id path segments; endpointErr reports an
	// invalid endpoint id on every call.
	segments    []string
	endpointErr error
}

func newQueueClient(endpointID string, cfg Config) queueClient {
	client, err := newTransportClient(cfg)
	segments, endpointErr := splitEndpointID(endpointID)
	return queueClient{cfg: cfg, client: client, clientErr: err, segments: segments, endpointErr: endpointErr}
}

// splitEndpointID validates an endpoint id such as "fal-ai/kling-video/o3/pro/image-to-video".
func splitEndpointID(endpointID string) ([]string, error) {
	segments := strings.Split(endpointID, "/")
	if len(segments) < 2 {
		return nil, fmt.Errorf("fal: endpoint id %q must have at least owner/app segments", endpointID)
	}
	for _, segment := range segments {
		if !validSegment(segment) {
			return nil, fmt.Errorf("fal: endpoint id %q has an empty or relative segment", endpointID)
		}
	}
	return segments, nil
}

// validSegment rejects segments that would vanish or traverse once a URL path
// is normalized.
func validSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".."
}

// endpointURL joins escaped segments beneath the base URL.
func (q queueClient) endpointURL(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return strings.TrimRight(q.cfg.BaseURL, "/") + "/" + strings.Join(escaped, "/")
}

// requestURL addresses a queued request under the endpoint's owner/app only,
// as fal's status and result routes ignore deeper endpoint segments.
func (q queueClient) requestURL(requestID string, suffix ...string) (string, error) {
	if q.endpointErr != nil {
		return "", q.endpointErr
	}
	if !validSegment(requestID) {
		return "", fmt.Errorf("fal: request id %q is empty or relative", requestID)
	}
	segments := append([]string{q.segments[0], q.segments[1], "requests", requestID}, suffix...)
	return q.endpointURL(segments...), nil
}

func (q queueClient) submitReply(ctx context.Context, input map[string]any) (QueueReply, error) {
	if q.endpointErr != nil {
		return QueueReply{}, q.endpointErr
	}
	if input == nil {
		input = map[string]any{}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return QueueReply{}, fmt.Errorf("fal: marshal submit input: %w", err)
	}
	return q.reply(ctx, http.MethodPost, q.endpointURL(q.segments...), body)
}

func (q queueClient) statusReply(ctx context.Context, requestID string) (QueueReply, error) {
	endpoint, err := q.requestURL(requestID, "status")
	if err != nil {
		return QueueReply{}, err
	}
	return q.reply(ctx, http.MethodGet, endpoint, nil)
}

func (q queueClient) resultReply(ctx context.Context, requestID string) (QueueReply, error) {
	endpoint, err := q.requestURL(requestID)
	if err != nil {
		return QueueReply{}, err
	}
	return q.reply(ctx, http.MethodGet, endpoint, nil)
}

func (q queueClient) reply(ctx context.Context, method, endpoint string, body []byte) (QueueReply, error) {
	if q.clientErr != nil {
		return QueueReply{}, fmt.Errorf("fal: configure transport: %w", q.clientErr)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := q.client.NewRequest(ctx, method, endpoint, reader)
	if err != nil {
		return QueueReply{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return QueueReply{}, err
	}
	defer resp.Body.Close()
	reply := QueueReply{StatusCode: resp.StatusCode}
	limit := q.cfg.MaxReplyBytes
	if limit > 0 && resp.ContentLength > limit {
		reply.ReadError = errors.New("reply body too large")
		return reply, nil
	}
	var bodyReader io.Reader = resp.Body
	if limit > 0 {
		bodyReader = io.LimitReader(bodyReader, limit+1)
	}
	reply.Body, err = io.ReadAll(bodyReader)
	switch {
	case err != nil:
		reply.ReadError = fmt.Errorf("reply body: %w", err)
	case limit > 0 && int64(len(reply.Body)) > limit:
		reply.ReadError = errors.New("reply body too large")
	case q.cfg.RequireUTF8Reply && !utf8.Valid(reply.Body):
		reply.ReadError = errors.New("reply body is not UTF-8")
	}
	return reply, nil
}

// SubmitReply POSTs input unchanged to {BaseURL}/{endpoint id} exactly once
// and returns the untranslated reply. It never retries an ambiguous submission.
func (m *VideoModel) SubmitReply(ctx context.Context, input map[string]any) (QueueReply, error) {
	return m.queue.submitReply(ctx, input)
}

// StatusReply GETs {BaseURL}/{owner}/{app}/requests/{id}/status once.
func (m *VideoModel) StatusReply(ctx context.Context, requestID string) (QueueReply, error) {
	return m.queue.statusReply(ctx, requestID)
}

// ResultReply GETs {BaseURL}/{owner}/{app}/requests/{id} once.
func (m *VideoModel) ResultReply(ctx context.Context, requestID string) (QueueReply, error) {
	return m.queue.resultReply(ctx, requestID)
}
