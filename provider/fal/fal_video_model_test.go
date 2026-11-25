package fal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestProviderRegistryMethods(t *testing.T) {
	t.Setenv("FAL_KEY", "env-key")
	provider := NewFromEnv()
	if provider.Name() != "fal" {
		t.Fatalf("Name() = %q", provider.Name())
	}
	cfg := provider.Config()
	if cfg.APIKey != "env-key" || cfg.BaseURL != "https://queue.fal.run" || cfg.Timeout != 120*time.Second {
		t.Fatalf("%+v", cfg)
	}
	if NewProvider("explicit").Config().APIKey != "explicit" {
		t.Fatal("explicit key did not win")
	}
	var registry llm.VideoProvider = provider
	if model := registry.VideoModel("fal-ai/veo3"); model.ModelID() != "fal-ai/veo3" {
		t.Fatalf("ModelID() = %q", model.ModelID())
	}
}

func TestVideoInputFromRequest(t *testing.T) {
	input, err := videoInput(llm.GenerateVideoRequest{
		Prompt:      "a cat",
		ImageURLs:   []string{"https://example.com/a.png"},
		Duration:    5 * time.Second,
		AspectRatio: "16:9",
		ProviderOptions: map[string]any{"fal": map[string]any{
			"rawInput": map[string]any{"aspect_ratio": "9:16", "generate_audio": true},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"aspect_ratio":"9:16","duration":"5","generate_audio":true,"image_url":"https://example.com/a.png","prompt":"a cat"}`
	if got, _ := json.Marshal(input); string(got) != want {
		t.Fatalf("%s", got)
	}
	input, err = videoInput(llm.GenerateVideoRequest{
		ImageURLs:       []string{"https://example.com/a.png", "https://example.com/b.png"},
		ProviderOptions: map[string]any{"fal": VideoOptions{}},
	})
	if err != nil || len(input) != 1 || len(input["image_urls"].([]string)) != 2 {
		t.Fatalf("%v %v", input, err)
	}
	input, err = videoInput(llm.GenerateVideoRequest{
		ImageURLs:       []string{"https://example.com/a.png"},
		ProviderOptions: map[string]any{"fal": &VideoOptions{RawInput: map[string]any{"image_urls": []string{"x"}}}},
	})
	if err != nil || input["image_url"] != nil {
		t.Fatalf("%v %v", input, err)
	}
	if _, err := videoInput(llm.GenerateVideoRequest{ProviderOptions: map[string]any{"fal": "bad"}}); err == nil {
		t.Fatal("want provider option type error")
	}
	if _, err := videoInput(llm.GenerateVideoRequest{Duration: -time.Second}); err == nil {
		t.Fatal("want negative duration error")
	}
}

func TestGenerateVideoPollsUntilCompleted(t *testing.T) {
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fal-ai/kling-video/o3/pro/image-to-video":
			fmt.Fprint(w, `{"status":"IN_QUEUE","request_id":"req-1"}`)
		case r.URL.Path == "/fal-ai/kling-video/requests/req-1/status":
			statusCalls++
			if statusCalls < 3 {
				w.WriteHeader(http.StatusAccepted)
				fmt.Fprint(w, `{"status":"IN_PROGRESS"}`)
				return
			}
			fmt.Fprint(w, `{"status":"COMPLETED"}`)
		case r.URL.Path == "/fal-ai/kling-video/requests/req-1":
			fmt.Fprint(w, `{"video":{"url":"https://v3b.fal.media/files/a.mp4"}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()
	model := NewProvider("key", WithConfig(Config{BaseURL: server.URL, PollInterval: time.Millisecond})).
		Video("fal-ai/kling-video/o3/pro/image-to-video")
	result, err := model.GenerateVideo(context.Background(), llm.GenerateVideoRequest{Prompt: "cat"})
	if err != nil || len(result.URLs) != 1 || result.URLs[0] != "https://v3b.fal.media/files/a.mp4" ||
		!strings.Contains(string(result.Raw), "a.mp4") || statusCalls != 3 {
		t.Fatalf("%+v %v calls=%d", result, err, statusCalls)
	}
}

func TestGenerateVideoReportsFailures(t *testing.T) {
	for _, tc := range []struct {
		name, statusBody, resultBody string
		resultStatus                 int
		want                         string
	}{
		{"status error", `{"status":"COMPLETED","error":"boom"}`, ``, 200, "fal: request req-1 failed: boom"},
		{"result detail", `{"status":"COMPLETED"}`, `{"detail":"policy"}`, 422, "fal: request req-1 failed: policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost:
					fmt.Fprint(w, `{"request_id":"req-1"}`)
				case strings.HasSuffix(r.URL.Path, "/status"):
					fmt.Fprint(w, tc.statusBody)
				default:
					w.WriteHeader(tc.resultStatus)
					fmt.Fprint(w, tc.resultBody)
				}
			}))
			defer server.Close()
			model := NewProvider("key", WithConfig(Config{BaseURL: server.URL, PollInterval: time.Millisecond})).
				Video("fal-ai/veo3")
			_, err := model.GenerateVideo(context.Background(), llm.GenerateVideoRequest{Prompt: "cat"})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestSubmitVideoReturnsSubmitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	model := NewProvider("key", WithBaseURL(server.URL)).Video("fal-ai/veo3")
	_, err := model.SubmitVideo(context.Background(), llm.GenerateVideoRequest{Prompt: "cat"})
	var submit *SubmitError
	if !errors.As(err, &submit) || !submit.Rejected || submit.Status != 429 {
		t.Fatalf("%v", err)
	}
}

func TestGenerateVideoPollTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `{"request_id":"req-1"}`)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"IN_QUEUE"}`)
	}))
	defer server.Close()
	model := NewProvider("key", WithConfig(Config{
		BaseURL: server.URL, PollInterval: time.Millisecond, PollTimeout: 30 * time.Millisecond,
	})).Video("fal-ai/veo3")
	_, err := model.GenerateVideo(context.Background(), llm.GenerateVideoRequest{Prompt: "cat"})
	if !errors.Is(err, errPollTimeout) {
		t.Fatalf("%v", err)
	}
}
