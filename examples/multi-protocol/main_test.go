package main

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
)

func TestAGUIEndpointRunsAgentWithStandardUserMessageID(t *testing.T) {
	assistant, err := agent.New(exampleModel{}).Build()
	if err != nil {
		t.Fatal(err)
	}
	run := func(ctx context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
		return assistant.Runner().Messages(messages...).Stream(ctx)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ag-ui", strings.NewReader(`{
		"threadId":"thread_1","runId":"run_1","state":{},
		"messages":[{"id":"user_1","role":"user","content":"hello"}],
		"tools":[],"context":[],"forwardedProps":{}
	}`))
	aisdkhttp.HandlerFor(agui.Protocol(), run).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"type":"RUN_FINISHED"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
