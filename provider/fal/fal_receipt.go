package fal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// QueueAcceptance identifies an accepted queue request.
type QueueAcceptance struct{ RequestID string }

// SubmitError classifies a rejected or indeterminate submission. Rejected is
// true only when fal definitely did not accept the request (an HTTP 4xx,
// including 429); otherwise the request may exist and must not be resubmitted
// blindly.
type SubmitError struct {
	Rejected bool
	Status   int
	Message  string
}

func (e *SubmitError) Error() string { return e.Message }

// QueueStatus is a decoded status reply. State is "pending" or "completed";
// ProviderState is fal's raw status (IN_QUEUE, IN_PROGRESS, COMPLETED).
// Error is set when a completed request reports a failure.
type QueueStatus struct {
	State         string
	ProviderState string
	Error         *string
}

// QueryError distinguishes contract violations, answers that will never
// become valid, from transient query failures worth retrying later.
type QueryError struct {
	Contract bool
	Message  string
}

func (e *QueryError) Error() string { return e.Message }

// VideoResult is a decoded result reply. State is "succeeded" (VideoURL set)
// or "failed" (Message optionally set).
type VideoResult struct {
	State    string
	VideoURL string
	Message  *string
}

const (
	previewLimit    = 300
	maxRequestIDLen = 256
)

// DecodeSubmission classifies a submit reply without retrying an uncertain request.
func DecodeSubmission(status int, body []byte, readErr error) (QueueAcceptance, error) {
	ambiguous := func(message string) (QueueAcceptance, error) {
		return QueueAcceptance{}, &SubmitError{Status: status, Message: message}
	}
	if status >= 500 && status < 600 {
		return ambiguous(fmt.Sprintf("submit answered HTTP %d", status))
	}
	if readErr != nil {
		return ambiguous("submit " + readErr.Error())
	}
	if status >= 400 && status < 500 {
		message := fmt.Sprintf("submit answered HTTP %d", status)
		if detail := failurePreview(body); detail != "" {
			message += ": " + detail
		}
		return QueueAcceptance{}, &SubmitError{Rejected: true, Status: status, Message: message}
	}
	if status < 200 || status >= 300 {
		return ambiguous(fmt.Sprintf("submit answered HTTP %d", status))
	}
	var reply struct {
		RequestID any `json:"request_id"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return ambiguous("submit reply is not a JSON object")
	}
	requestID, ok := reply.RequestID.(string)
	requestID = strings.TrimSpace(requestID)
	if !ok || !validSegment(requestID) || len(requestID) > maxRequestIDLen {
		return ambiguous("submit reply has no request_id")
	}
	return QueueAcceptance{RequestID: requestID}, nil
}

// DecodeStatus classifies a status reply. 404 (unknown request) and unknown
// status values are contract errors; 429, 5xx, other non-2xx answers, read
// errors and non-JSON bodies are transient.
func DecodeStatus(status int, body []byte, readErr error) (QueueStatus, error) {
	fail := func(contract bool, message string) (QueueStatus, error) {
		return QueueStatus{}, &QueryError{Contract: contract, Message: message}
	}
	if status == http.StatusNotFound {
		return fail(true, "status answered HTTP 404: the request is unknown")
	}
	if status < 200 || status >= 300 {
		return fail(false, fmt.Sprintf("status answered HTTP %d", status))
	}
	if readErr != nil {
		return fail(false, "status "+readErr.Error())
	}
	var reply struct {
		Status    *string `json:"status"`
		Error     any     `json:"error"`
		ErrorType any     `json:"error_type"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return fail(false, "status reply is not a JSON object")
	}
	if reply.Status == nil {
		return fail(true, "status reply has no status")
	}
	switch state := *reply.Status; state {
	case "IN_QUEUE", "IN_PROGRESS":
		return QueueStatus{State: "pending", ProviderState: state}, nil
	case "COMPLETED":
		out := QueueStatus{State: "completed", ProviderState: state}
		message, ok := looseText(reply.Error)
		if !ok {
			message, ok = looseText(reply.ErrorType)
		}
		if ok {
			message = preview(message, previewLimit)
			out.Error = &message
		}
		return out, nil
	default:
		return fail(true, fmt.Sprintf("status %s is unknown", preview(state, 64)))
	}
}

// DecodeVideoResult classifies a result reply. A 2xx reply must carry an
// https `video.url`; a 4xx reply other than 429 is the model's failure. 429,
// 5xx, other answers, read errors and non-JSON 2xx bodies are transient.
//
// Fetch the result only after DecodeStatus reports "completed": fal answers
// a result request for a running job with a 4xx as well.
func DecodeVideoResult(status int, body []byte, readErr error) (VideoResult, error) {
	fail := func(contract bool, message string) (VideoResult, error) {
		return VideoResult{}, &QueryError{Contract: contract, Message: message}
	}
	if status == http.StatusTooManyRequests || status < 200 || status >= 500 ||
		(status >= 300 && status < 400) {
		return fail(false, fmt.Sprintf("result answered HTTP %d", status))
	}
	if readErr != nil {
		return fail(false, "result "+readErr.Error())
	}
	if status >= 400 {
		message := failurePreview(body)
		if message == "" {
			message = fmt.Sprintf("result answered HTTP %d", status)
		}
		return VideoResult{State: "failed", Message: &message}, nil
	}
	var reply struct {
		Video *struct {
			URL any `json:"url"`
		} `json:"video"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return fail(false, "result reply is not a JSON object")
	}
	if reply.Video == nil {
		return fail(true, "result has no video")
	}
	videoURL, ok := reply.Video.URL.(string)
	if !ok || !isHTTPSURL(videoURL) {
		return fail(true, "the result video URL is not an https URL")
	}
	return VideoResult{State: "succeeded", VideoURL: videoURL}, nil
}

// failurePreview prefers the `detail` field of a JSON error body and falls
// back to the trimmed body itself.
func failurePreview(body []byte) string {
	var reply struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(body, &reply) == nil {
		if text, ok := looseText(reply.Detail); ok {
			return preview(text, previewLimit)
		}
	}
	return preview(strings.TrimSpace(string(body)), previewLimit)
}

// looseText renders a string as-is and any other non-null JSON value compactly.
func looseText(raw any) (string, bool) {
	switch v := raw.(type) {
	case nil:
		return "", false
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func isHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != ""
}

// preview truncates text to at most limit runes.
func preview(text string, limit int) string {
	for index := range text {
		if limit == 0 {
			return text[:index]
		}
		limit--
	}
	return text
}
