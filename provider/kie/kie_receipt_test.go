package kie

import (
	"errors"
	"testing"
)

func TestImageReceiptPreservesTaskAndNumericFailure(t *testing.T) {
	id, err := DecodeImageSubmission([]byte(`{"code":402,"data":{"taskId":" task "}}`))
	if err != nil || id != " task " {
		t.Fatalf("%q %v", id, err)
	}
	state, err := DecodeImageStatus(
		"task",
		[]byte(`{"code":200,"data":{"state":"FAIL","failCode":422,"failMsg":"bad"}}`),
	)
	if err != nil || state.State != "fail" || state.FailCode == nil || *state.FailCode != 422 ||
		state.FailMessage == nil ||
		*state.FailMessage != "bad" {
		t.Fatalf("%+v %v", state, err)
	}
	for _, body := range []string{`{"data":{"state":"success","resultJson":{"resultUrls":["https://example.com/a"]}}}`, `{"data":{"state":"success","resultJson":"{\"resultUrls\":[\"https://example.com/a\"]}"}}`} {
		state, err = DecodeImageStatus("task", []byte(body))
		if err != nil || len(state.ResultURLs) != 1 || state.ResultURLs[0] != "https://example.com/a" {
			t.Fatalf("%+v %v", state, err)
		}
	}
}

func TestVideoReceiptPreservesFailureCreditsAndClassification(t *testing.T) {
	state, err := DecodeVideoStatus(
		"task",
		"custom",
		200,
		[]byte(`{"code":200,"data":{"state":"fail","failCode":422,"failMsg":"bad","creditsConsumed":"1.25"}}`),
		nil,
	)
	if err != nil || state.State != "failed" || state.Code == nil || *state.Code != "422" ||
		state.CreditsConsumed == nil ||
		*state.CreditsConsumed != 1.25 {
		t.Fatalf("%+v %v", state, err)
	}
	for _, tc := range []struct {
		status   int
		body     string
		rejected bool
	}{
		{503, `{"code":429}`, false}, {200, `{"code":429,"msg":"busy"}`, true}, {200, `{"code":200,"data":{}}`, false},
	} {
		_, err := DecodeVideoSubmission(tc.status, []byte(tc.body), nil)
		var submit *JobsSubmitError
		if !errors.As(err, &submit) || submit.Rejected != tc.rejected || submit.Status != tc.status {
			t.Fatalf("%v", err)
		}
	}
}
