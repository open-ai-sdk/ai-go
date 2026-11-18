package kie

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ImageTaskState preserves numeric failure codes and optional failure text.
type ImageTaskState struct {
	State       string
	ResultURLs  []string
	FailCode    *int64
	FailMessage *string
}

// DecodeImageSubmission decodes a jobs image submission without rejecting an
// otherwise valid task id solely because of its envelope code.
func DecodeImageSubmission(body []byte) (string, error) {
	var parsed struct {
		Code int64   `json:"code"`
		Msg  *string `json:"msg"`
		Data *struct {
			TaskID *string `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to decode kie createTask response: %w", err)
	}
	if parsed.Data == nil {
		return "", fmt.Errorf("kie createTask returned no data (code=%d)", parsed.Code)
	}
	if parsed.Data.TaskID == nil || strings.TrimSpace(*parsed.Data.TaskID) == "" {
		return "", fmt.Errorf("kie createTask response missing taskId (code=%d)", parsed.Code)
	}
	return *parsed.Data.TaskID, nil
}

// DecodeImageStatus decodes object or string resultJson and numeric failCode.
func DecodeImageStatus(taskID string, body []byte) (ImageTaskState, error) {
	var parsed struct {
		Code int64   `json:"code"`
		Msg  *string `json:"msg"`
		Data *struct {
			State   *string         `json:"state"`
			Result  json.RawMessage `json:"resultJson"`
			Code    *int64          `json:"failCode"`
			Message *string         `json:"failMsg"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ImageTaskState{}, err
	}
	if parsed.Data == nil || parsed.Data.State == nil {
		return ImageTaskState{}, fmt.Errorf("kie recordInfo for task %s had no state", taskID)
	}
	d := parsed.Data
	state := strings.ToLower(*d.State)
	result := ImageTaskState{State: state, FailCode: d.Code, FailMessage: d.Message}
	if state == "success" {
		result.ResultURLs = []string{}
		if len(d.Result) > 0 && string(d.Result) != "null" {
			raw := d.Result
			if raw[0] == '"' {
				var text string
				if err := json.Unmarshal(raw, &text); err != nil {
					return result, err
				}
				raw = []byte(text)
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return result, errors.New("unexpected resultJson shape: null")
			}
			var output struct {
				URLs json.RawMessage `json:"resultUrls"`
			}
			if err := json.Unmarshal(raw, &output); err != nil {
				return result, fmt.Errorf("failed to decode kie resultJson: %w", err)
			}
			if len(output.URLs) > 0 {
				if string(output.URLs) == "null" {
					return result, errors.New("failed to decode kie resultJson: resultUrls is null")
				}
				if err := json.Unmarshal(output.URLs, &result.ResultURLs); err != nil {
					return result, fmt.Errorf("failed to decode kie resultJson: %w", err)
				}
			}
		}
	}
	return result, nil
}
