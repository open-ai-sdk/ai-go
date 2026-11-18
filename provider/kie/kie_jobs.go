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
	"strings"

	"github.com/open-ai-sdk/ai-go/transport"
)

// TaskState is the normalized lifecycle state of a Kie jobs-API task.
type TaskState string

const (
	// TaskStateUnknown is reported for any raw state the SDK does not recognize.
	TaskStateUnknown TaskState = "unknown"
	// TaskStateWaiting means the task was accepted and has not been scheduled.
	TaskStateWaiting TaskState = "waiting"
	// TaskStateQueuing means the task is queued upstream.
	TaskStateQueuing TaskState = "queuing"
	// TaskStateGenerating means the task is running.
	TaskStateGenerating TaskState = "generating"
	// TaskStateSuccess is terminal: result URLs are available.
	TaskStateSuccess TaskState = "success"
	// TaskStateFail is terminal: FailCode/FailMsg describe the failure.
	TaskStateFail TaskState = "fail"
)

// Terminal reports whether the state is final (success or fail).
func (s TaskState) Terminal() bool { return s == TaskStateSuccess || s == TaskStateFail }

// Pending reports whether the task is still progressing.
func (s TaskState) Pending() bool {
	return s == TaskStateWaiting || s == TaskStateQueuing || s == TaskStateGenerating
}

// normalizeState maps a raw upstream state string onto a [TaskState].
func normalizeState(s string) TaskState {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success", "succeed", "succeeded":
		return TaskStateSuccess
	case "fail", "failed", "error":
		return TaskStateFail
	case "waiting":
		return TaskStateWaiting
	case "queuing", "queued":
		return TaskStateQueuing
	case "generating", "processing", "running":
		return TaskStateGenerating
	default:
		return TaskStateUnknown
	}
}

// TaskRecord is the parsed result of `GET /api/v1/jobs/recordInfo`.
type TaskRecord struct {
	TaskID string
	// State is the normalized state; RawState is the upstream string verbatim.
	State    TaskState
	RawState string
	// ResultURLs are the result URLs parsed from resultUrls or resultJson.
	ResultURLs []string
	FailCode   string
	FailMsg    string
	// Raw is the exact successful response body for diagnostics. It may
	// contain sensitive provider data and is never logged automatically.
	Raw json.RawMessage `json:"-"`
}

func newTaskRecord(data recordInfoData) TaskRecord {
	return TaskRecord{
		TaskID:     data.TaskID,
		State:      normalizeState(data.State),
		RawState:   data.State,
		ResultURLs: data.URLs(),
		FailCode:   data.FailCode,
		FailMsg:    data.FailMsg,
		Raw:        data.raw,
	}
}

// TaskOption customizes a [Provider.CreateTask] call.
type TaskOption func(*createTaskRequest)

// WithCallBackURL sets the top-level `callBackUrl` Kie notifies on completion.
func WithCallBackURL(callBackURL string) TaskOption {
	return func(r *createTaskRequest) { r.CallBackURL = callBackURL }
}

// jobsClient talks to the Kie unified jobs API (createTask / recordInfo).
type jobsClient struct {
	cfg       Config
	client    *transport.Client
	clientErr error
}

func newJobsClient(cfg Config) jobsClient {
	client, err := newTransportClient(cfg)
	return jobsClient{cfg: cfg, client: client, clientErr: err}
}

// createTask POSTs `/api/v1/jobs/createTask` and returns the task id.
func (j jobsClient) createTask(
	ctx context.Context,
	model string,
	input map[string]any,
	opts ...TaskOption,
) (string, error) {
	if model == "" {
		return "", errors.New("kie: createTask: model is required")
	}
	if input == nil {
		input = map[string]any{}
	}
	payload := createTaskRequest{Model: model, Input: input}
	for _, opt := range opts {
		opt(&payload)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("kie: marshal createTask: %w", err)
	}
	if j.clientErr != nil {
		return "", fmt.Errorf("kie: configure transport: %w", j.clientErr)
	}
	httpReq, err := j.client.NewRequest(
		ctx, http.MethodPost, buildKieURL(&j.cfg, "/api/v1/jobs/createTask"),
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("kie: build createTask request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, status, err := doHTTP(j.client, httpReq)
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
		return "", &KieError{
			Code: parsed.Code, Msg: parsed.Msg, Status: status, RawBody: string(respBody),
		}
	}
	return parsed.Data.TaskID, nil
}

// taskStatus calls `GET /api/v1/jobs/recordInfo?taskId=...` once.
func (j jobsClient) taskStatus(ctx context.Context, taskID string) (TaskRecord, error) {
	if taskID == "" {
		return TaskRecord{}, errors.New("kie: recordInfo: task id is required")
	}
	if j.clientErr != nil {
		return TaskRecord{}, fmt.Errorf("kie: configure transport: %w", j.clientErr)
	}
	endpoint := buildKieURL(&j.cfg, "/api/v1/jobs/recordInfo") + "?taskId=" + url.QueryEscape(taskID)
	httpReq, err := j.client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TaskRecord{}, fmt.Errorf("kie: build recordInfo request: %w", err)
	}
	respBody, status, err := doHTTP(j.client, httpReq)
	if err != nil {
		return TaskRecord{}, err
	}
	if status != http.StatusOK {
		return TaskRecord{}, parseKieError(status, respBody)
	}
	var parsed recordInfoResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return TaskRecord{}, fmt.Errorf("kie: parse recordInfo response: %w", err)
	}
	if parsed.Code != 200 {
		return TaskRecord{}, &KieError{
			Code: parsed.Code, Msg: parsed.Msg, Status: status, RawBody: string(respBody),
		}
	}
	parsed.Data.raw = append(json.RawMessage(nil), respBody...)
	return newTaskRecord(parsed.Data), nil
}

// await polls taskStatus until the task succeeds, fails, or the poll budget
// is exhausted.
func (j jobsClient) await(ctx context.Context, taskID string) (TaskRecord, error) {
	return poll(
		ctx,
		poller{Interval: j.cfg.PollInterval, MaxWait: j.cfg.PollTimeout},
		func(ctx context.Context) (TaskRecord, bool, error) {
			rec, err := j.taskStatus(ctx, taskID)
			if err != nil {
				return TaskRecord{}, false, err
			}
			switch {
			case rec.State == TaskStateSuccess:
				return rec, true, nil
			case rec.State == TaskStateFail:
				msg := rec.FailMsg
				if msg == "" {
					msg = rec.RawState
				}
				return TaskRecord{}, false, fmt.Errorf("kie: task %s failed: %s", taskID, msg)
			case rec.State.Pending():
				return TaskRecord{}, false, nil
			default:
				// Unknown state: treat as terminal failure to avoid forever-loops.
				return TaskRecord{}, false, fmt.Errorf(
					"kie: task %s unknown state %q", taskID, rec.RawState,
				)
			}
		},
	)
}

// CreateTask submits an arbitrary jobs-API task and returns its task id
// without waiting. input is sent verbatim as the `input` envelope.
func (p *Provider) CreateTask(
	ctx context.Context,
	model string,
	input map[string]any,
	opts ...TaskOption,
) (string, error) {
	return p.jobs.createTask(ctx, model, input, opts...)
}

// TaskStatus fetches the current state of a task once, without polling.
func (p *Provider) TaskStatus(ctx context.Context, taskID string) (TaskRecord, error) {
	return p.jobs.taskStatus(ctx, taskID)
}

// doHTTP runs req, returning body + status. Errors at the transport layer are
// returned verbatim so callers (and tests) can match on them.
func doHTTP(client transport.Doer, req *http.Request) ([]byte, int, error) {
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
