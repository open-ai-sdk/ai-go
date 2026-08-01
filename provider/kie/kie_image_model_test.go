package kie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// fakeKieServer wires the three endpoints we need (createTask, recordInfo,
// image download) onto one httptest.Server so tests can run end-to-end without
// real network.
type fakeKieServer struct {
	server      *httptest.Server
	pollCalls   int32
	pollsToWait int32
	imageBytes  []byte
	lastBody    []byte
	lastModel   string
	lastInput   map[string]any
	authHeader  string
	httpResult  bool // when true, recordInfo returns an http:// result URL
}

func newFakeKieServer(t *testing.T, pollsToWait int) *fakeKieServer {
	t.Helper()
	f := &fakeKieServer{
		pollsToWait: int32(pollsToWait),
		imageBytes:  []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, // jpeg-ish header
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/jobs/createTask", func(w http.ResponseWriter, r *http.Request) {
		f.authHeader = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		f.lastBody = body
		var parsed createTaskRequest
		_ = json.Unmarshal(body, &parsed)
		f.lastModel = parsed.Model
		f.lastInput = parsed.Input
		_ = json.NewEncoder(w).Encode(createTaskResponse{
			Code: 200,
			Msg:  "ok",
			Data: struct {
				TaskID string `json:"taskId"`
			}{TaskID: "task-fake-1"},
		})
	})

	mux.HandleFunc("/api/v1/jobs/recordInfo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("taskId") != "task-fake-1" {
			http.Error(w, "bad task", http.StatusBadRequest)
			return
		}
		n := atomic.AddInt32(&f.pollCalls, 1)
		state := "processing"
		var resultURLs []string
		if n > f.pollsToWait {
			state = "success"
			if f.httpResult {
				resultURLs = []string{f.server.URL + "/result/img1.jpg"}
			} else {
				// Use https://api.kie.ai/... so the HTTPS-only download
				// guard passes; rewriteTransport (set by tests) reroutes
				// the host to the fake httptest server.
				resultURLs = []string{"https://api.kie.ai/result/img1.jpg"}
			}
		}
		_ = json.NewEncoder(w).Encode(recordInfoResponse{
			Code: 200,
			Msg:  "ok",
			Data: recordInfoData{
				TaskID:     "task-fake-1",
				State:      state,
				ResultURLs: resultURLs,
			},
		})
	})

	mux.HandleFunc("/result/img1.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(f.imageBytes)
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// TestGenerate_TextToImage_Success validates the full submit→poll→download
// cycle using nano-banana-2 (no input images required).
//
// NOTE: server URL is http://127.0.0.1, so we override the downloadImages
// HTTPS-only check by reusing the same fake host for the result URL. The
// production build retains the HTTPS-only guard — see TestGenerate_RejectsHTTP.
func TestGenerate_TextToImage_Success(t *testing.T) {
	srv := newFakeKieServer(t, 2)

	cfg := Config{
		APIKey:       "test-key",
		BaseURL:      "", // direct mode
		PollInterval: 1 * time.Millisecond,
		PollTimeout:  2 * time.Second,
		HTTPClient: &http.Client{Transport: rewriteTransport{
			from: "https://api.kie.ai",
			to:   srv.server.URL,
			base: http.DefaultTransport,
		}},
	}
	model := newImageModel(ModelNanoBanana2, cfg.resolved())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := model.Generate(ctx, ai.GenerateImageRequest{
		Prompt: "a cat",
	})
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(res.Images))
	}
	if !bytesEqual(res.Images[0].Data, srv.imageBytes) {
		t.Errorf("downloaded bytes mismatch")
	}
	if srv.lastModel != "nano-banana-2" {
		t.Errorf("model=%q", srv.lastModel)
	}
	if srv.lastInput["prompt"] != "a cat" {
		t.Errorf("input.prompt=%v", srv.lastInput["prompt"])
	}
	if srv.authHeader != "Bearer test-key" {
		t.Errorf("auth=%q", srv.authHeader)
	}
	if atomic.LoadInt32(&srv.pollCalls) != 3 {
		t.Errorf("pollCalls=%d, want 3 (2 processing + 1 success)", srv.pollCalls)
	}
}

func TestGenerate_RejectsHTTP(t *testing.T) {
	srv := newFakeKieServer(t, 0)
	srv.httpResult = true
	cfg := Config{
		APIKey: "k", PollInterval: time.Millisecond, PollTimeout: time.Second,
		HTTPClient: &http.Client{Transport: rewriteTransport{
			from: "https://api.kie.ai", to: srv.server.URL, base: http.DefaultTransport,
		}},
	}.resolved()
	model := newImageModel(ModelNanoBanana2, cfg)
	_, err := model.Generate(context.Background(), ai.GenerateImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Errorf("err = %v; want non-https rejection", err)
	}
}

func TestGenerate_ImageToImagePreservesURLs(t *testing.T) {
	srv := newFakeKieServer(t, 0)
	cfg := Config{
		APIKey: "k", PollInterval: time.Millisecond, PollTimeout: 2 * time.Second,
		HTTPClient: &http.Client{Transport: rewriteTransport{
			from: "https://api.kie.ai", to: srv.server.URL, base: http.DefaultTransport,
		}},
	}.resolved()
	model := newImageModel(ModelGPTImage2ImageToImage, cfg)
	_, _ = model.Generate(context.Background(), ai.GenerateImageRequest{
		Prompt: "edit",
		Images: []ai.ImageInput{{URL: "https://cdn/in1.png"}, {URL: "https://cdn/in2.png"}},
	})
	urls, _ := srv.lastInput["input_urls"].([]any)
	if len(urls) != 2 {
		t.Fatalf("input_urls=%v", srv.lastInput["input_urls"])
	}
	if urls[0] != "https://cdn/in1.png" || urls[1] != "https://cdn/in2.png" {
		t.Errorf("input_urls=%v", urls)
	}
}

func TestNormalizeState(t *testing.T) {
	cases := map[string]taskState{
		"success":    stateSuccess,
		"SUCCESS":    stateSuccess,
		"succeeded":  stateSuccess,
		"fail":       stateFail,
		"FAILED":     stateFail,
		"error":      stateFail,
		"waiting":    statePending,
		"queuing":    statePending,
		"processing": statePending,
		"":           stateUnknown,
		"weird":      stateUnknown,
	}
	for in, want := range cases {
		if got := normalizeState(in); got != want {
			t.Errorf("normalizeState(%q)=%v, want %v", in, got, want)
		}
	}
}

// rewriteTransport rewrites the request URL host so we can keep buildKieURL
// pointed at the canonical upstream while tests run against httptest.
type rewriteTransport struct {
	from string
	to   string
	base http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if u := req.URL.String(); strings.HasPrefix(u, r.from) {
		newURL := r.to + strings.TrimPrefix(u, r.from)
		nr, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		nr.Header = req.Header
		return r.base.RoundTrip(nr)
	}
	return r.base.RoundTrip(req)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
