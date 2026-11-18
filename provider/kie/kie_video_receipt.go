package kie

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// JobsVideoAcceptance identifies an accepted durable task.
type JobsVideoAcceptance struct{ TaskID string }

// JobsVideoState contains the verified video task outcome.
type JobsVideoState struct {
	State           string
	VideoURL        string
	CreditsConsumed *float64
	Code, Message   *string
	ProviderState   string
}

// JobsSubmitError classifies a rejected or indeterminate submission.
type JobsSubmitError struct {
	Rejected bool
	Code     *int64
	Message  string
	Status   int
}

func (e *JobsSubmitError) Error() string { return e.Message }

// JobsQueryError distinguishes contract violations from transient query failures.
type JobsQueryError struct {
	Contract bool
	Message  string
}

func (e *JobsQueryError) Error() string { return e.Message }

type videoEnvelope struct {
	Code    *int64         `json:"code"`
	Message *string        `json:"msg"`
	Data    map[string]any `json:"data"`
}

// DecodeVideoSubmission classifies a jobs reply without retrying an uncertain task.
func DecodeVideoSubmission(status int, body []byte, readErr error) (JobsVideoAcceptance, error) {
	ambiguous := func(message string) (JobsVideoAcceptance, error) {
		return JobsVideoAcceptance{}, &JobsSubmitError{Message: message, Status: status}
	}
	if status >= 500 && status < 600 {
		return ambiguous(fmt.Sprintf("createTask answered HTTP %d", status))
	}
	if readErr != nil {
		return ambiguous("createTask " + readErr.Error())
	}
	if status >= 400 && status < 500 {
		code := int64(status)
		return JobsVideoAcceptance{}, &JobsSubmitError{
			Rejected: true,
			Code:     &code,
			Message:  jobsPreview(string(body), 300),
			Status:   status,
		}
	}
	if status < 200 || status >= 300 {
		return ambiguous(fmt.Sprintf("createTask answered HTTP %d", status))
	}
	var envelope videoEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return ambiguous("createTask reply is not the jobs envelope")
	}
	if envelope.Code == nil {
		return ambiguous("createTask reply has no code")
	}
	code := *envelope.Code
	if code == 200 {
		taskID, ok := envelope.Data["taskId"].(string)
		if !ok {
			return ambiguous("createTask code 200 without a taskId")
		}
		taskID = strings.TrimSpace(taskID)
		if taskID == "" || len(taskID) > 256 {
			return ambiguous("createTask code 200 without a taskId")
		}
		return JobsVideoAcceptance{taskID}, nil
	}
	message := ""
	if envelope.Message != nil {
		message = jobsPreview(*envelope.Message, 300)
	}
	switch code {
	case 400, 401, 402, 404, 422, 429, 433, 455, 505:
		return JobsVideoAcceptance{}, &JobsSubmitError{Rejected: true, Code: &code, Message: message, Status: status}
	}
	return ambiguous(fmt.Sprintf("createTask code %d: %s", code, message))
}

// DecodeVideoStatus verifies task/model identity and decodes a single-result video task.
func DecodeVideoStatus(taskID, model string, status int, body []byte, readErr error) (JobsVideoState, error) {
	fail := func(contract bool, message string) (JobsVideoState, error) {
		return JobsVideoState{}, &JobsQueryError{contract, message}
	}
	if status < 200 || status >= 300 {
		return fail(false, fmt.Sprintf("recordInfo answered HTTP %d", status))
	}
	if readErr != nil {
		return fail(false, "recordInfo "+readErr.Error())
	}
	var envelope videoEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return fail(false, "recordInfo reply is not the jobs envelope")
	}
	if envelope.Code == nil {
		return fail(false, "recordInfo reply has no code")
	}
	code := *envelope.Code
	if code != 200 {
		message := ""
		if envelope.Message != nil {
			message = jobsPreview(*envelope.Message, 300)
		}
		return fail(code == 404 || code == 422, fmt.Sprintf("recordInfo code %d: %s", code, message))
	}
	d := envelope.Data
	if d == nil {
		return fail(true, "recordInfo code 200 without data")
	}
	if id, ok := d["taskId"].(string); ok && id != taskID {
		return fail(true, fmt.Sprintf("recordInfo answered for another task (%s)", id))
	}
	if value, ok := d["model"].(string); ok && value != model {
		return fail(true, fmt.Sprintf("recordInfo task is model %s, not %s", value, model))
	}
	return decodeVideoState(d)
}

func decodeVideoState(d map[string]any) (JobsVideoState, error) {
	fail := func(message string) (JobsVideoState, error) {
		return JobsVideoState{}, &JobsQueryError{true, message}
	}
	credits := jobsLooseNumber(d["creditsConsumed"])
	state, ok := d["state"].(string)
	if !ok {
		return fail("recordInfo data has no state")
	}
	switch state {
	case "waiting", "queuing", "generating":
		return JobsVideoState{State: "pending", ProviderState: state}, nil
	case "success":
		endpoint, err := DecodeVideoResultURL(d["resultJson"])
		if err != nil {
			return JobsVideoState{}, err
		}
		return JobsVideoState{State: "succeeded", VideoURL: endpoint, CreditsConsumed: credits}, nil
	case "fail":
		var codePtr, messagePtr *string
		if code, ok := jobsLooseString(d["failCode"]); ok {
			codePtr = &code
		}
		if message, ok := d["failMsg"].(string); ok && strings.TrimSpace(message) != "" {
			message = jobsPreview(message, 300)
			messagePtr = &message
		}
		return JobsVideoState{State: "failed", Code: codePtr, Message: messagePtr, CreditsConsumed: credits}, nil
	}
	return fail(fmt.Sprintf("recordInfo state %s is unknown", state))
}

// DecodeVideoResultURL verifies the jobs video resultJson string and its HTTPS URL.
func DecodeVideoResultURL(raw any) (string, error) {
	fail := func(message string) (string, error) { return "", &JobsQueryError{true, message} }
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fail("a succeeded task has no resultJson string")
	}
	if len(text) > 64*1024 {
		return fail("resultJson is too large")
	}
	var result map[string]any
	if json.Unmarshal([]byte(text), &result) != nil || result == nil {
		return fail("resultJson is not a JSON object")
	}
	for key := range result {
		if key != "resultUrls" && key != "originUrls" {
			return fail(fmt.Sprintf("resultJson has an unknown field %s: the result shape is not verified", key))
		}
	}
	urls, ok := result["resultUrls"].([]any)
	if !ok {
		return fail("resultJson has no resultUrls list")
	}
	if len(urls) != 1 {
		return fail("resultJson must hold exactly one result URL")
	}
	endpoint, ok := urls[0].(string)
	if !ok {
		return fail("resultJson must hold exactly one result URL")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return fail("the result URL is not an https URL")
	}
	return endpoint, nil
}

func jobsLooseNumber(raw any) *float64 {
	var n float64
	switch v := raw.(type) {
	case float64:
		n = v
	case string:
		var err error
		n, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil
		}
	default:
		return nil
	}
	return &n
}

func jobsLooseString(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	}
	return "", false
}

func jobsPreview(text string, limit int) string {
	for index := range text {
		if limit == 0 {
			return text[:index]
		}
		limit--
	}
	return text
}
