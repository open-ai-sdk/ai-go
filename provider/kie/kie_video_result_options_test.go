package kie

import (
	"errors"
	"testing"
)

const extraFieldSuccess = `{"code":200,"data":{"taskId":"t1","model":"bytedance/seedance-2-5","state":"success",` +
	`"creditsConsumed":42,"resultJson":"{\"resultUrls\":[\"https://cdn.example/v.mp4\"],` +
	`\"lastFrameUrl\":\"https://cdn.example/f.jpg\",\"seed\":7}"}}`

func TestVideoStatusDefaultRejectsUnknownResultFields(t *testing.T) {
	_, err := DecodeVideoStatus("t1", "bytedance/seedance-2-5", 200, []byte(extraFieldSuccess), nil)
	var queryErr *JobsQueryError
	if !errors.As(err, &queryErr) || !queryErr.Contract {
		t.Fatalf("want a contract error for unknown result fields, got %v", err)
	}
}

func TestVideoStatusWithExtraFieldsReturnsVideoAndLastFrame(t *testing.T) {
	state, err := DecodeVideoStatusWithOptions("t1", "bytedance/seedance-2-5", 200,
		[]byte(extraFieldSuccess), nil, VideoResultOptions{AllowExtraFields: true})
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "succeeded" || state.VideoURL != "https://cdn.example/v.mp4" ||
		state.LastFrameURL != "https://cdn.example/f.jpg" || state.CreditsConsumed == nil || *state.CreditsConsumed != 42 {
		t.Fatalf("unexpected state %+v", state)
	}
}

func TestVideoStatusWithExtraFieldsDropsNonHTTPSLastFrame(t *testing.T) {
	body := `{"code":200,"data":{"taskId":"t1","model":"m","state":"success",` +
		`"resultJson":"{\"resultUrls\":[\"https://cdn.example/v.mp4\"],\"lastFrameUrl\":\"http://x/f.jpg\"}"}}`
	state, err := DecodeVideoStatusWithOptions(
		"t1",
		"m",
		200,
		[]byte(body),
		nil,
		VideoResultOptions{AllowExtraFields: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastFrameURL != "" {
		t.Fatalf("a non-https frame must be dropped, got %q", state.LastFrameURL)
	}
}

func TestVideoStatusWithExtraFieldsStillRequiresHTTPSVideo(t *testing.T) {
	body := `{"code":200,"data":{"taskId":"t1","model":"m","state":"success",` +
		`"resultJson":"{\"resultUrls\":[\"http://cdn.example/v.mp4\"]}"}}`
	_, err := DecodeVideoStatusWithOptions(
		"t1",
		"m",
		200,
		[]byte(body),
		nil,
		VideoResultOptions{AllowExtraFields: true},
	)
	if err == nil {
		t.Fatal("want an error for a non-https video URL")
	}
}
