package kie

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// UploadOptions controls Kie file-upload helpers. UploadPath is required by
// the upstream API; FileName is optional and lets callers control the URL slug.
type UploadOptions struct {
	// UploadPath is the destination folder, no leading or trailing slash
	// (e.g. "images/user-uploads"). Required by the upstream API.
	UploadPath string

	// FileName is optional; when empty, Kie auto-generates one.
	FileName string
}

// UploadBase64 sends a Base64-encoded file to `POST /api/file-base64-upload`.
// `data` may be either a raw Base64 string or a `data:<mime>;base64,<...>`
// URI — Kie accepts both.
//
// Files have a 3-day TTL upstream; callers should consume the URL promptly.
func (p *Provider) UploadBase64(ctx context.Context, data string, opts UploadOptions) (string, error) {
	body, err := json.Marshal(struct {
		Base64Data string `json:"base64Data"`
		UploadPath string `json:"uploadPath"`
		FileName   string `json:"fileName,omitempty"`
	}{Base64Data: data, UploadPath: opts.UploadPath, FileName: opts.FileName})
	if err != nil {
		return "", fmt.Errorf("kie: marshal base64 upload: %w", err)
	}

	url := buildKieURL(&p.cfg, "/api/file-base64-upload")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("kie: build base64 upload request: %w", err)
	}
	p.applyHeaders(req, "application/json")

	return p.doUpload(req)
}

// UploadStream sends a binary blob via multipart/form-data to
// `POST /api/file-stream-upload`. This is a convenience wrapper around
// UploadFromReader for callers that already have the data in memory.
//
// `filename` is sent as the multipart `filename=` and, when opts.FileName is
// empty, also as the `fileName` field.
func (p *Provider) UploadStream(ctx context.Context, blob []byte, filename string, opts UploadOptions) (string, error) {
	return p.UploadFromReader(ctx, bytes.NewReader(blob), filename, opts)
}

// UploadFromReader streams a file via multipart/form-data to
// `POST /api/file-stream-upload` without buffering the entire body in memory.
// It uses an io.Pipe so the multipart.Writer writes directly into the HTTP
// request body.
//
// `filename` is sent as the multipart `filename=` and, when opts.FileName is
// empty, also as the `fileName` field.
func (p *Provider) UploadFromReader(
	ctx context.Context, r io.Reader, filename string, opts UploadOptions,
) (string, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType()

	// Write multipart form data in a goroutine; the HTTP request reads
	// from the pipe reader concurrently.
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()

		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("kie: multipart file part: %w", err))
			errCh <- err
			return
		}
		if _, err := io.Copy(fw, r); err != nil {
			pw.CloseWithError(fmt.Errorf("kie: copy file part: %w", err))
			errCh <- err
			return
		}
		if err := mw.WriteField("uploadPath", opts.UploadPath); err != nil {
			pw.CloseWithError(fmt.Errorf("kie: multipart uploadPath: %w", err))
			errCh <- err
			return
		}
		if opts.FileName != "" {
			if err := mw.WriteField("fileName", opts.FileName); err != nil {
				pw.CloseWithError(fmt.Errorf("kie: multipart fileName: %w", err))
				errCh <- err
				return
			}
		}
		if err := mw.Close(); err != nil {
			pw.CloseWithError(fmt.Errorf("kie: multipart close: %w", err))
			errCh <- err
			return
		}
		errCh <- nil
	}()

	url := buildKieURL(&p.cfg, "/api/file-stream-upload")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		pr.Close()
		return "", fmt.Errorf("kie: build stream upload request: %w", err)
	}
	p.applyHeaders(req, contentType)

	result, uploadErr := p.doUpload(req)

	// Wait for the writer goroutine to finish; surface its error only if
	// the upload itself succeeded (upload errors take precedence).
	if writeErr := <-errCh; writeErr != nil && uploadErr == nil {
		return "", writeErr
	}
	return result, uploadErr
}

// doUpload runs an upload request and unwraps the standard
// {success, code, msg, data} envelope to a downloadUrl.
func (p *Provider) doUpload(req *http.Request) (string, error) {
	respBody, status, err := doHTTP(p.cfg.HTTPClient, req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", parseKieError(status, respBody)
	}

	var parsed kieFileUploadResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("kie: parse upload response: %w", err)
	}
	if parsed.Code != 200 || parsed.Data.DownloadURL == "" {
		return "", &KieError{Code: parsed.Code, Msg: parsed.Msg, Status: status, RawBody: string(respBody)}
	}
	return parsed.Data.DownloadURL, nil
}

// applyHeaders mirrors ImageModel.applyHeaders for Provider-level requests.
func (p *Provider) applyHeaders(req *http.Request, contentType string) {
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	for k, v := range p.cfg.Headers {
		req.Header.Set(k, v)
	}
}
