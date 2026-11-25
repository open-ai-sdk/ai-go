package fal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueueRepliesBuildURLsAndAuthenticate(t *testing.T) {
	for _, tc := range []struct {
		endpoint, submitPath, statusPath, resultPath string
	}{
		{
			"fal-ai/veo3",
			"/base/fal-ai/veo3",
			"/base/fal-ai/veo3/requests/req%201/status",
			"/base/fal-ai/veo3/requests/req%201",
		},
		{
			"fal-ai/kling-video/o3/pro/image-to-video",
			"/base/fal-ai/kling-video/o3/pro/image-to-video",
			"/base/fal-ai/kling-video/requests/req%201/status",
			"/base/fal-ai/kling-video/requests/req%201",
		},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.Method+" "+r.URL.EscapedPath())
				if r.Header.Get("Authorization") != "Key secret" || r.Header.Get("X-Extra") != "1" {
					t.Errorf("headers: %v", r.Header)
				}
				if r.Method == http.MethodPost {
					if r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("content type: %q", r.Header.Get("Content-Type"))
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["prompt"] != "cat" {
						t.Errorf("body: %v %v", body, err)
					}
				}
				w.WriteHeader(http.StatusAccepted)
				fmt.Fprint(w, `{"status":"IN_QUEUE"}`)
			}))
			defer server.Close()
			model := NewProvider("secret", WithConfig(Config{
				BaseURL: server.URL + "/base/",
				Headers: map[string]string{"X-Extra": "1"},
			})).Video(tc.endpoint)
			ctx := context.Background()
			reply, err := model.SubmitReply(ctx, map[string]any{"prompt": "cat"})
			if err != nil || reply.StatusCode != 202 || string(reply.Body) != `{"status":"IN_QUEUE"}` {
				t.Fatalf("%+v %v", reply, err)
			}
			if _, err := model.StatusReply(ctx, "req 1"); err != nil {
				t.Fatal(err)
			}
			if _, err := model.ResultReply(ctx, "req 1"); err != nil {
				t.Fatal(err)
			}
			want := []string{"POST " + tc.submitPath, "GET " + tc.statusPath, "GET " + tc.resultPath}
			if strings.Join(paths, "\n") != strings.Join(want, "\n") {
				t.Fatalf("paths:\n%s\nwant:\n%s", strings.Join(paths, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestQueueRepliesEscapeSegments(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
	}))
	defer server.Close()
	model := NewProvider("k", WithBaseURL(server.URL)).Video("owner/app?x/v#1")
	if _, err := model.StatusReply(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if got != "/owner/app%3Fx/requests/a%2Fb/status" {
		t.Fatalf("path: %s", got)
	}
}

func TestQueueRepliesRejectInvalidIDs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	p := NewProvider("k", WithBaseURL(server.URL))
	ctx := context.Background()
	for _, endpoint := range []string{"", "fal-ai", "fal-ai/", "/veo3", "fal-ai//veo3", "fal-ai/../x"} {
		model := p.Video(endpoint)
		if _, err := model.SubmitReply(ctx, nil); err == nil {
			t.Errorf("submit %q: want error", endpoint)
		}
		if _, err := model.StatusReply(ctx, "id"); err == nil {
			t.Errorf("status %q: want error", endpoint)
		}
		if _, err := model.ResultReply(ctx, "id"); err == nil {
			t.Errorf("result %q: want error", endpoint)
		}
	}
	model := p.Video("fal-ai/veo3")
	for _, id := range []string{"", ".", ".."} {
		if _, err := model.StatusReply(ctx, id); err == nil {
			t.Errorf("status id %q: want error", id)
		}
		if _, err := model.ResultReply(ctx, id); err == nil {
			t.Errorf("result id %q: want error", id)
		}
	}
	if calls != 0 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestQueueReplyOptionalLimits(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		utf8       bool
		want       string
	}{
		{"default", "abcdefgh", 0, false, ""},
		{"exact", "abcd", 4, false, ""},
		{"bounded", "abcdefgh", 4, false, "reply body too large"},
		{"invalidUTF8", string([]byte{255}), 10, true, "reply body is not UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }),
			)
			defer server.Close()
			model := NewProvider("key", WithConfig(Config{
				BaseURL: server.URL, MaxReplyBytes: tc.limit, RequireUTF8Reply: tc.utf8,
			})).Video("fal-ai/veo3")
			reply, err := model.StatusReply(context.Background(), "id")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if reply.ReadError != nil || string(reply.Body) != tc.body {
					t.Fatalf("%+v", reply)
				}
				return
			}
			if reply.ReadError == nil || reply.ReadError.Error() != tc.want || reply.StatusCode != 200 {
				t.Fatalf("%+v", reply)
			}
		})
	}
}

func TestQueueReplyCapsStreamedBodyWithoutContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc")
		w.(http.Flusher).Flush() // chunked: no Content-Length
		fmt.Fprint(w, "defgh")
	}))
	defer server.Close()
	model := NewProvider("key", WithConfig(Config{BaseURL: server.URL, MaxReplyBytes: 4})).Video("fal-ai/veo3")
	reply, err := model.ResultReply(context.Background(), "id")
	if err != nil || reply.ReadError == nil || reply.ReadError.Error() != "reply body too large" {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestQueueReplyDoesNotFollowRedirectWhenConfigured(t *testing.T) {
	for _, injected := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
		}))
		cfg := Config{BaseURL: server.URL, DisableRedirects: true}
		var client *http.Client
		if injected {
			client = &http.Client{}
			cfg.HTTPClient = client
		}
		model := NewProvider("key", WithConfig(cfg)).Video("fal-ai/veo3")
		reply, err := model.SubmitReply(context.Background(), map[string]any{})
		server.Close()
		if err != nil || reply.StatusCode != 307 || calls != 1 {
			t.Fatalf("injected=%v: %+v %v calls=%d", injected, reply, err, calls)
		}
		if injected && client.CheckRedirect != nil {
			t.Fatal("injected client was mutated")
		}
	}
}
