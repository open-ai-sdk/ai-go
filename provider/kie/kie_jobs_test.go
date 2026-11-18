package kie

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/llm"
)

// jobsServer is a scriptable createTask/recordInfo fake. records are served in
// order; the last one repeats.
type jobsServer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	records []string // raw `data` JSON objects
	polls   int32
	body    createTaskRequest
	auth    string
}

func newJobsServer(t *testing.T, records ...string) *jobsServer {
	t.Helper()
	j := &jobsServer{records: records}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/kie/jobs/createTask", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		j.mu.Lock()
		j.auth = r.Header.Get("Authorization")
		_ = json.Unmarshal(b, &j.body)
		j.mu.Unlock()
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"taskId":"t-1"}}`))
	})
	mux.HandleFunc("/api/kie/jobs/recordInfo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("taskId") != "t-1" {
			http.Error(w, "bad task", http.StatusBadRequest)
			return
		}
		n := int(atomic.AddInt32(&j.polls, 1)) - 1
		if n >= len(j.records) {
			n = len(j.records) - 1
		}
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":` + j.records[n] + `}`))
	})
	j.srv = httptest.NewServer(mux)
	t.Cleanup(j.srv.Close)
	return j
}

func (j *jobsServer) provider() *Provider {
	return NewProvider("k", WithConfig(Config{
		APIKey: "k", BaseURL: j.srv.URL, PollInterval: time.Millisecond, PollTimeout: time.Second,
	}))
}

func (j *jobsServer) sent() createTaskRequest {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.body
}

func TestProviderCreateTaskAndTaskStatusSplit(t *testing.T) {
	const raw = `{"taskId":"t-1","state":"success","resultJson":"{\"resultUrls\":[\"https://cdn/a.mp4\"]}","failCode":"","failMsg":""}`
	j := newJobsServer(t, raw)
	p := j.provider()
	ctx := context.Background()

	id, err := p.CreateTask(ctx, "vendor/any-model", map[string]any{"prompt": "hi"}, WithCallBackURL("https://cb"))
	if err != nil || id != "t-1" {
		t.Fatalf("CreateTask = %q, %v", id, err)
	}
	got := j.sent()
	if got.Model != "vendor/any-model" || got.Input["prompt"] != "hi" || got.CallBackURL != "https://cb" {
		t.Fatalf("sent = %+v", got)
	}
	if j.auth != "Bearer k" {
		t.Fatalf("auth = %q", j.auth)
	}
	if atomic.LoadInt32(&j.polls) != 0 {
		t.Fatal("CreateTask must not poll")
	}

	rec, err := p.TaskStatus(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != TaskStateSuccess || rec.RawState != "success" || rec.TaskID != "t-1" {
		t.Fatalf("rec = %+v", rec)
	}
	if !reflect.DeepEqual(rec.ResultURLs, []string{"https://cdn/a.mp4"}) {
		t.Fatalf("urls = %v", rec.ResultURLs)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Raw, &env); err != nil || string(env.Data) != raw {
		t.Fatalf("Raw not retained: %s", rec.Raw)
	}
}

func TestTaskStatusFailureFieldsAndErrors(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"fail","failCode":"E42","failMsg":"nope"}`)
	p := j.provider()
	rec, err := p.TaskStatus(context.Background(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != TaskStateFail || rec.FailCode != "E42" || rec.FailMsg != "nope" {
		t.Fatalf("rec = %+v", rec)
	}
	if !rec.State.Terminal() || rec.State.Pending() {
		t.Fatal("fail must be terminal and not pending")
	}

	var kerr *KieError
	_, err = p.TaskStatus(context.Background(), "other")
	if !errors.As(err, &kerr) || kerr.Status != http.StatusBadRequest {
		t.Fatalf("err = %v, want KieError 400", err)
	}
	if _, err := p.TaskStatus(context.Background(), ""); err == nil {
		t.Fatal("empty task id must error")
	}
	if _, err := p.CreateTask(context.Background(), "", nil); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestTaskStatePendingTerminal(t *testing.T) {
	for _, s := range []TaskState{TaskStateWaiting, TaskStateQueuing, TaskStateGenerating} {
		if !s.Pending() || s.Terminal() {
			t.Errorf("%s: pending/terminal wrong", s)
		}
	}
	if TaskStateUnknown.Pending() || TaskStateUnknown.Terminal() {
		t.Error("unknown is neither pending nor terminal")
	}
	if !TaskStateSuccess.Terminal() {
		t.Error("success must be terminal")
	}
}

func TestImageUnknownModelPassthrough(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"success","resultUrls":["https://api.kie.ai/r.png"]}`)
	p := j.provider()
	model := p.Image("vendor/brand-new-image")

	taskID, err := model.submitTask(context.Background(), llm.GenerateImageRequest{
		Prompt: "a cat",
		Images: []llm.ImageInput{{URL: "https://cdn/in.png"}},
		ProviderOptions: map[string]any{"kie": ImageOptions{
			RawInput:    map[string]any{"style": "flat", "prompt": "override"},
			Extra:       map[string]any{"seed": 7},
			CallBackURL: "https://cb",
		}},
	})
	if err != nil || taskID != "t-1" {
		t.Fatalf("submitTask = %q, %v", taskID, err)
	}
	got := j.sent()
	if got.Model != "vendor/brand-new-image" || got.CallBackURL != "https://cb" {
		t.Fatalf("sent = %+v", got)
	}
	if got.Input["style"] != "flat" || got.Input["prompt"] != "override" || got.Input["seed"] != float64(7) {
		t.Fatalf("input = %v", got.Input)
	}
	if urls, _ := got.Input["image_urls"].([]any); len(urls) != 1 || urls[0] != "https://cdn/in.png" {
		t.Fatalf("image_urls = %v", got.Input["image_urls"])
	}
}

func TestImageUnknownModelPassthroughFillsPromptAndMapOptions(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"success"}`)
	model := j.provider().Image("vendor/x")
	_, err := model.submitTask(context.Background(), llm.GenerateImageRequest{
		Prompt:          "p",
		ProviderOptions: map[string]any{"kie": map[string]any{"rawInput": map[string]any{"k": "v"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := j.sent().Input
	if in["prompt"] != "p" || in["k"] != "v" {
		t.Fatalf("input = %v", in)
	}
	if _, ok := in["image_urls"]; ok {
		t.Fatal("image_urls must be omitted when no images")
	}
}

func TestImageListedModelIgnoresRawInput(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"success"}`)
	model := j.provider().Image(ModelNanoBanana2)
	_, err := model.submitTask(context.Background(), llm.GenerateImageRequest{
		Prompt:          "p",
		ProviderOptions: map[string]any{"kie": ImageOptions{RawInput: map[string]any{"zzz": 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := j.sent().Input["zzz"]; ok {
		t.Fatal("typed models must not receive RawInput")
	}
}

func TestVideoSubmitStatusSplit(t *testing.T) {
	j := newJobsServer(t,
		`{"taskId":"t-1","state":"queuing"}`,
		`{"taskId":"t-1","state":"success","resultJson":"{\"resultUrls\":[\"https://cdn/v.mp4\"]}"}`,
	)
	vm := j.provider().Video("vendor/video-x")
	ctx := context.Background()

	id, err := vm.SubmitVideo(ctx, llm.GenerateVideoRequest{
		Prompt:      "waves",
		ImageURLs:   []string{"https://cdn/first.png"},
		Duration:    5 * time.Second,
		AspectRatio: "16:9",
		ProviderOptions: map[string]any{"kie": VideoOptions{
			RawInput:    map[string]any{"sound": true},
			CallBackURL: "https://cb",
		}},
	})
	if err != nil || id != "t-1" {
		t.Fatalf("SubmitVideo = %q, %v", id, err)
	}
	got := j.sent()
	if got.Model != "vendor/video-x" || got.CallBackURL != "https://cb" {
		t.Fatalf("sent = %+v", got)
	}
	want := map[string]any{
		"prompt": "waves", "image_urls": []any{"https://cdn/first.png"},
		"duration": "5", "aspect_ratio": "16:9", "sound": true,
	}
	if !reflect.DeepEqual(got.Input, want) {
		t.Fatalf("input = %v, want %v", got.Input, want)
	}
	if atomic.LoadInt32(&j.polls) != 0 {
		t.Fatal("SubmitVideo must not poll")
	}

	rec, err := vm.VideoStatus(ctx, id)
	if err != nil || rec.State != TaskStateQueuing {
		t.Fatalf("status1 = %+v, %v", rec, err)
	}
	rec, err = vm.VideoStatus(ctx, id)
	if err != nil || rec.State != TaskStateSuccess || len(rec.ResultURLs) != 1 {
		t.Fatalf("status2 = %+v, %v", rec, err)
	}
}

func TestVideoRawInputWinsOverRequestFields(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"success"}`)
	vm := j.provider().Video("m")
	_, err := vm.SubmitVideo(context.Background(), llm.GenerateVideoRequest{
		Prompt: "p", Duration: 8 * time.Second, AspectRatio: "1:1",
		ProviderOptions: map[string]any{"kie": map[string]any{
			"rawInput": map[string]any{"duration": 10, "aspect_ratio": "9:16"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := j.sent().Input
	if in["duration"] != float64(10) || in["aspect_ratio"] != "9:16" {
		t.Fatalf("input = %v", in)
	}
}

func TestVideoSubmitValidation(t *testing.T) {
	vm := newJobsServer(t, `{"taskId":"t-1","state":"success"}`).provider().Video("m")
	if _, err := vm.SubmitVideo(context.Background(), llm.GenerateVideoRequest{Duration: -time.Second}); err == nil {
		t.Fatal("negative duration must error")
	}
	if _, err := vm.SubmitVideo(context.Background(), llm.GenerateVideoRequest{
		ProviderOptions: map[string]any{"kie": 42},
	}); err == nil {
		t.Fatal("bad provider options must error")
	}
}

func TestGenerateVideoPollsToSuccess(t *testing.T) {
	j := newJobsServer(t,
		`{"taskId":"t-1","state":"waiting"}`,
		`{"taskId":"t-1","state":"generating"}`,
		`{"taskId":"t-1","state":"success","resultUrls":["https://cdn/v.mp4"]}`,
	)
	vm := j.provider().VideoModel("vendor/v")
	res, err := vm.GenerateVideo(context.Background(), llm.GenerateVideoRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.URLs, []string{"https://cdn/v.mp4"}) || len(res.Raw) == 0 {
		t.Fatalf("res = %+v", res)
	}
	if atomic.LoadInt32(&j.polls) != 3 {
		t.Fatalf("polls = %d, want 3", j.polls)
	}
	if vm.ModelID() != "vendor/v" {
		t.Fatalf("ModelID = %q", vm.ModelID())
	}
}

func TestGenerateVideoFailureAndEmptyResult(t *testing.T) {
	cases := []struct {
		name, record, wantErr string
	}{
		{"fail", `{"taskId":"t-1","state":"fail","failMsg":"policy"}`, "failed: policy"},
		{"empty", `{"taskId":"t-1","state":"success"}`, "no result URLs"},
		{"unknown", `{"taskId":"t-1","state":"weird"}`, "unknown state"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := newJobsServer(t, tc.record).provider().Video("m")
			_, err := vm.GenerateVideo(context.Background(), llm.GenerateVideoRequest{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestGenerateVideoContextCancel(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"generating"}`)
	p := NewProvider("k", WithConfig(Config{
		APIKey: "k", BaseURL: j.srv.URL, PollInterval: 5 * time.Millisecond, PollTimeout: 5 * time.Second,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for atomic.LoadInt32(&j.polls) < 2 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	_, err := p.Video("m").GenerateVideo(ctx, llm.GenerateVideoRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestGenerateVideoPollTimeout(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"generating"}`)
	p := NewProvider("k", WithConfig(Config{
		APIKey: "k", BaseURL: j.srv.URL, PollInterval: time.Millisecond, PollTimeout: 30 * time.Millisecond,
	}))
	_, err := p.Video("m").GenerateVideo(context.Background(), llm.GenerateVideoRequest{})
	if !errors.Is(err, errPollTimeout) {
		t.Fatalf("err = %v, want errPollTimeout", err)
	}
}

func TestCreateTaskContextCancel(t *testing.T) {
	j := newJobsServer(t, `{"taskId":"t-1","state":"success"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := j.provider().CreateTask(ctx, "m", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProviderImplementsVideoCapability(t *testing.T) {
	var _ llm.VideoProvider = NewProvider("k")
}
