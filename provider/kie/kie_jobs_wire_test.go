package kie

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJobsReplyDirectWireAndRawNumbers(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authentication")
		}
		if r.Method == http.MethodPost {
			if r.URL.Path != "/api/v1/jobs/createTask" {
				t.Errorf("path: %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			input := body["input"].(map[string]any)
			if input["duration"] != float64(8) || input["generation_type"] != "TEXT_2_VIDEO" {
				t.Errorf("input: %#v", input)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"code":500,"msg":"uncertain"}`)
			return
		}
		if r.URL.Path != "/api/v1/jobs/recordInfo" || r.URL.Query().Get("taskId") != "a +/&" {
			t.Errorf("URL: %s", r.URL)
		}
		fmt.Fprint(w, `{"code":200,"data":{"state":"fail","failCode":42,"creditsConsumed":"1.25"}}`)
	}))
	defer server.Close()
	p := NewProvider("secret", WithConfig(Config{BaseURL: server.URL, DirectBaseURL: true}))
	model := p.Video("custom-model")
	reply, err := model.SubmitVideoReply(
		context.Background(),
		map[string]any{"duration": 8, "generation_type": "TEXT_2_VIDEO"},
	)
	if err != nil || reply.ReadError != nil || reply.StatusCode != 503 || calls != 1 {
		t.Fatalf("%+v %v calls=%d", reply, err, calls)
	}
	reply, err = model.VideoStatusReply(context.Background(), "a +/&")
	if err != nil || reply.ReadError != nil || !strings.Contains(string(reply.Body), `"failCode":42`) ||
		!strings.Contains(string(reply.Body), `"creditsConsumed":"1.25"`) {
		t.Fatalf("%+v %v", reply, err)
	}
}

func TestJobsReplyOptionalLimits(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		cap        int64
		utf8       bool
		want       string
	}{
		{"default", "abcdefgh", 0, false, ""},
		{"bounded", "abcdefgh", 4, false, "reply body too large"},
		{"invalidUTF8", string([]byte{255}), 10, true, "reply body is not UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }),
			)
			defer server.Close()
			p := NewProvider(
				"key",
				WithConfig(
					Config{BaseURL: server.URL, DirectBaseURL: true, MaxReplyBytes: tc.cap, RequireUTF8Reply: tc.utf8},
				),
			)
			reply, err := p.TaskStatusReply(context.Background(), "task")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if reply.ReadError != nil {
					t.Fatal(reply.ReadError)
				}
				return
			}
			if reply.ReadError == nil || reply.ReadError.Error() != tc.want || reply.StatusCode != 200 {
				t.Fatalf("%+v", reply)
			}
		})
	}
}

func TestJobsReplyDoesNotFollowRedirectWhenConfigured(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	p := NewProvider("key", WithConfig(Config{BaseURL: server.URL, DirectBaseURL: true, DisableRedirects: true}))
	reply, err := p.CreateTaskReply(context.Background(), "custom", map[string]any{})
	if err != nil || reply.StatusCode != 307 || calls != 1 {
		t.Fatalf("%+v %v calls=%d", reply, err, calls)
	}
}
