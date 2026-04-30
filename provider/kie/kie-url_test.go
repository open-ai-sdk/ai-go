package kie

import "testing"

func TestBuildKieURL_NoBaseURL(t *testing.T) {
	cfg := &Config{}
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/jobs/createTask", "https://api.kie.ai/api/v1/jobs/createTask"},
		{"/api/v1/jobs/recordInfo", "https://api.kie.ai/api/v1/jobs/recordInfo"},
		{"/api/file-base64-upload", "https://api.kie.ai/api/file-base64-upload"},
		{"/api/file-stream-upload", "https://api.kie.ai/api/file-stream-upload"},
	}
	for _, tt := range tests {
		if got := buildKieURL(cfg, tt.path); got != tt.want {
			t.Errorf("buildKieURL(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

func TestBuildKieURL_WithProxy(t *testing.T) {
	cfg := &Config{BaseURL: "https://gen.example.com"}
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/jobs/createTask", "https://gen.example.com/api/kie/jobs/createTask"},
		{"/api/v1/jobs/recordInfo", "https://gen.example.com/api/kie/jobs/recordInfo"},
		{"/api/file-base64-upload", "https://gen.example.com/api/kie/file/base64-upload"},
		{"/api/file-stream-upload", "https://gen.example.com/api/kie/file/stream-upload"},
	}
	for _, tt := range tests {
		if got := buildKieURL(cfg, tt.path); got != tt.want {
			t.Errorf("buildKieURL(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

func TestBuildKieURL_TrimsTrailingSlash(t *testing.T) {
	cfg := &Config{BaseURL: "https://gen.example.com/"}
	got := buildKieURL(cfg, "/api/v1/jobs/createTask")
	want := "https://gen.example.com/api/kie/jobs/createTask"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}
