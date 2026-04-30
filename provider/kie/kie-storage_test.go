package kie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadBase64_Success(t *testing.T) {
	var seen struct {
		Base64Data string `json:"base64Data"`
		UploadPath string `json:"uploadPath"`
		FileName   string `json:"fileName"`
	}
	var auth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file-base64-upload" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seen)
		_ = json.NewEncoder(w).Encode(kieFileUploadResponse{
			Success: true, Code: 200, Msg: "ok",
			Data: struct {
				FileName    string `json:"fileName"`
				FilePath    string `json:"filePath"`
				DownloadURL string `json:"downloadUrl"`
				FileSize    int64  `json:"fileSize"`
				MimeType    string `json:"mimeType"`
				UploadedAt  string `json:"uploadedAt"`
			}{
				FileName:    "out.png",
				FilePath:    "uploads/out.png",
				DownloadURL: "https://tempfile.redpandaai.co/uploads/out.png",
				FileSize:    100,
				MimeType:    "image/png",
				UploadedAt:  "2025-01-01T00:00:00.000Z",
			},
		})
	}))
	defer srv.Close()

	p := NewProvider("api-key", WithBaseURL(srv.URL+"_NOT_USED"))
	// Override to talk directly to the test server (bypassing buildKieURL's
	// /api/kie/file/... rewrite); easiest: set BaseURL="" and rewrite host.
	p.cfg.BaseURL = ""
	p.cfg.HTTPClient.Transport = rewriteTransport{
		from: "https://api.kie.ai", to: srv.URL, base: http.DefaultTransport,
	}

	url, err := p.UploadBase64(context.Background(), "dGVzdA==", UploadOptions{
		UploadPath: "images/test", FileName: "out.png",
	})
	if err != nil {
		t.Fatalf("UploadBase64 err: %v", err)
	}
	if url != "https://tempfile.redpandaai.co/uploads/out.png" {
		t.Errorf("url=%q", url)
	}
	if seen.Base64Data != "dGVzdA==" || seen.UploadPath != "images/test" || seen.FileName != "out.png" {
		t.Errorf("body: %+v", seen)
	}
	if auth != "Bearer api-key" {
		t.Errorf("auth=%q", auth)
	}
}

func TestUploadStream_Multipart(t *testing.T) {
	var contentType string
	var bodyContains struct {
		hasFile    bool
		hasUpload  bool
		hasFilName bool
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		bodyContains.hasFile = strings.Contains(s, `name="file"`)
		bodyContains.hasUpload = strings.Contains(s, `name="uploadPath"`)
		bodyContains.hasFilName = strings.Contains(s, `name="fileName"`)
		_ = json.NewEncoder(w).Encode(kieFileUploadResponse{
			Success: true, Code: 200, Msg: "ok",
			Data: struct {
				FileName    string `json:"fileName"`
				FilePath    string `json:"filePath"`
				DownloadURL string `json:"downloadUrl"`
				FileSize    int64  `json:"fileSize"`
				MimeType    string `json:"mimeType"`
				UploadedAt  string `json:"uploadedAt"`
			}{DownloadURL: "https://tempfile.redpandaai.co/x.bin"},
		})
	}))
	defer srv.Close()

	p := NewProvider("k")
	p.cfg.HTTPClient.Transport = rewriteTransport{
		from: "https://api.kie.ai", to: srv.URL, base: http.DefaultTransport,
	}

	url, err := p.UploadStream(context.Background(), []byte("hello"), "blob.bin", UploadOptions{
		UploadPath: "images/u", FileName: "blob.bin",
	})
	if err != nil {
		t.Fatalf("UploadStream err: %v", err)
	}
	if url != "https://tempfile.redpandaai.co/x.bin" {
		t.Errorf("url=%q", url)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Errorf("Content-Type=%q", contentType)
	}
	if !bodyContains.hasFile || !bodyContains.hasUpload || !bodyContains.hasFilName {
		t.Errorf("body parts missing: %+v", bodyContains)
	}
}

func TestUploadBase64_ErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"msg":"bad path"}`))
	}))
	defer srv.Close()

	p := NewProvider("k")
	p.cfg.HTTPClient.Transport = rewriteTransport{
		from: "https://api.kie.ai", to: srv.URL, base: http.DefaultTransport,
	}

	_, err := p.UploadBase64(context.Background(), "x", UploadOptions{UploadPath: "p"})
	if err == nil {
		t.Fatal("err = nil")
	}
	ke, ok := err.(*KieError)
	if !ok {
		t.Fatalf("err type=%T", err)
	}
	if ke.Code != 400 || ke.Msg != "bad path" {
		t.Errorf("got %+v", ke)
	}
}
