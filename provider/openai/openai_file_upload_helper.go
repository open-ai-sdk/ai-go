package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/open-ai-sdk/ai-go/transport"
)

// UploadedFile holds the metadata returned by the OpenAI /v1/files endpoint
// after a successful upload.
type UploadedFile struct {
	// ID is the OpenAI file identifier (e.g. "file-abc123").
	ID string
	// Object is always "file" for successfully uploaded files.
	Object string
	// Filename is the name provided during upload.
	Filename string
	// Bytes is the size of the uploaded file in bytes.
	Bytes int
	// Status is the processing status: "uploaded", "processed", or "error".
	Status string
}

// FilePurpose identifies the intended use of an uploaded file.
type FilePurpose string

const (
	// FilePurposeUserData is used for files passed to model inputs via file_id.
	FilePurposeUserData FilePurpose = "user_data"
	// FilePurposeAssistants is used for files associated with Assistants.
	FilePurposeAssistants FilePurpose = "assistants"
	// FilePurposeFineTune is used for fine-tuning dataset files.
	FilePurposeFineTune FilePurpose = "fine-tune"
	// FilePurposeBatch is used for Batch API input files.
	FilePurposeBatch FilePurpose = "batch"
	// FilePurposeVision is used for vision fine-tuning.
	FilePurposeVision FilePurpose = "vision"
)

// UploadFileRequest holds the parameters for a file upload.
type UploadFileRequest struct {
	// Filename is the name to assign the file on the OpenAI platform.
	Filename string
	// Purpose identifies the intended use of the file.
	Purpose FilePurpose
	// Data is the raw file content to upload.
	Data []byte
	// MediaType is the MIME type of the file (e.g. "application/pdf").
	// Defaults to "application/octet-stream" when empty.
	MediaType string
}

// UploadFile uploads data to the OpenAI /v1/files endpoint. File operations
// belong to the provider Client because they are not tied to a model ID.
func (c *Client) UploadFile(ctx context.Context, req UploadFileRequest) (*UploadedFile, error) {
	if err := validateUploadFileRequest(req); err != nil {
		return nil, err
	}
	return c.uploadFile(ctx, req)
}

// UploadFile is retained for source compatibility. New code should call
// Client.UploadFile.
func (m *LanguageModel) UploadFile(ctx context.Context, req UploadFileRequest) (*UploadedFile, error) {
	if err := validateUploadFileRequest(req); err != nil {
		return nil, err
	}
	if m.clientErr != nil {
		return nil, fmt.Errorf("openai: upload file: configure transport: %w", m.clientErr)
	}
	return m.client.uploadFile(ctx, req)
}

func validateUploadFileRequest(req UploadFileRequest) error {
	if req.Filename == "" {
		return fmt.Errorf("openai: upload file: filename is required")
	}
	if req.Purpose == "" {
		return fmt.Errorf("openai: upload file: purpose is required")
	}
	if len(req.Data) == 0 {
		return fmt.Errorf("openai: upload file: data is empty")
	}
	return nil
}

func (c *Client) uploadFile(ctx context.Context, req UploadFileRequest) (*UploadedFile, error) {

	mimeType := req.MediaType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	body, contentType, err := buildMultipartBody(req.Filename, req.Purpose, req.Data, mimeType)
	if err != nil {
		return nil, fmt.Errorf("openai: upload file: build multipart: %w", err)
	}

	httpReq, err := c.uploads.NewRequest(ctx, http.MethodPost, "files", body)
	if err != nil {
		return nil, fmt.Errorf("openai: upload file: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := c.uploads.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: upload file: http: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, transport.APIErrorFromResponse(ctx, "openai-file-upload", resp)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: upload file: read response: %w", err)
	}

	return parseFileResponse(respBody)
}

// buildMultipartBody constructs the multipart/form-data body for the /v1/files endpoint.
func buildMultipartBody(
	filename string, purpose FilePurpose, data []byte, mimeType string,
) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Add "purpose" field.
	if err := w.WriteField("purpose", string(purpose)); err != nil {
		return nil, "", err
	}

	// Add "file" part with explicit Content-Type header.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", mimeType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err = part.Write(data); err != nil {
		return nil, "", err
	}

	if err = w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

// fileResponse mirrors the JSON object returned by the OpenAI Files API.
type fileResponse struct {
	ID       string `json:"id"`
	Object   string `json:"object"`
	Filename string `json:"filename"`
	Bytes    int    `json:"bytes"`
	Status   string `json:"status"`
	Error    *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func parseFileResponse(body []byte) (*UploadedFile, error) {
	var r fileResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("openai: upload file: decode response: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("openai: upload file: %s: %s", r.Error.Code, r.Error.Message)
	}
	return &UploadedFile{
		ID:       r.ID,
		Object:   r.Object,
		Filename: r.Filename,
		Bytes:    r.Bytes,
		Status:   r.Status,
	}, nil
}
