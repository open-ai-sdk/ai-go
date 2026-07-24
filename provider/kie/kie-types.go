package kie

import "encoding/json"

// createTaskRequest is the wire shape of `POST /api/v1/jobs/createTask`.
//
// All three v1 image models share this envelope; only the `model`
// discriminator and the contents of `input` differ.
type createTaskRequest struct {
	Model       string         `json:"model"`
	Input       map[string]any `json:"input"`
	CallBackURL string         `json:"callBackUrl,omitempty"`
}

// createTaskResponse is the wire shape returned by createTask. The `data.taskId`
// is the handle used by the recordInfo polling endpoint.
type createTaskResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

// recordInfoResponse is the wire shape of `GET /api/v1/jobs/recordInfo?taskId=`.
//
// The polling endpoint follows Kie's standard `{code, msg, data}` envelope
// (same as createTask and the upload endpoints).
//
// `state` enum (per Kie convention): "waiting" | "queuing" | "processing" |
// "success" | "fail". Anything else is treated as terminal-failed to avoid
// poll-forever loops.
//
// Result URLs may arrive in either `resultUrls` (array) or as a JSON-encoded
// string in `resultJson` (e.g. `{"resultUrls":["https://..."]}`). Both shapes
// are normalised by recordInfoData.URLs.
type recordInfoResponse struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data recordInfoData `json:"data"`
}

type recordInfoData struct {
	TaskID     string          `json:"taskId"`
	State      string          `json:"state"`
	FailCode   string          `json:"failCode,omitempty"`
	FailMsg    string          `json:"failMsg,omitempty"`
	ResultJSON string          `json:"resultJson,omitempty"`
	ResultURLs []string        `json:"resultUrls,omitempty"`
	Param      json.RawMessage `json:"param,omitempty"`
}

// ResultURLs returns the result URLs regardless of which field the upstream
// populated. The `resultJson` string field, when present, is parsed for a
// `resultUrls` array.
func (d recordInfoData) URLs() []string {
	if len(d.ResultURLs) > 0 {
		return d.ResultURLs
	}
	if d.ResultJSON == "" {
		return nil
	}
	var parsed struct {
		ResultURLs []string `json:"resultUrls"`
	}
	if err := json.Unmarshal([]byte(d.ResultJSON), &parsed); err != nil {
		return nil
	}
	return parsed.ResultURLs
}

// kieFileUploadResponse is the wire shape of `/api/file-base64-upload` and
// `/api/file-stream-upload`.
type kieFileUploadResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		FileName    string `json:"fileName"`
		FilePath    string `json:"filePath"`
		DownloadURL string `json:"downloadUrl"`
		FileSize    int64  `json:"fileSize"`
		MediaType   string `json:"mimeType"`
		UploadedAt  string `json:"uploadedAt"`
	} `json:"data"`
}
