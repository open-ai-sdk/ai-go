package fal

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeSubmissionClassification(t *testing.T) {
	readErr := errors.New("reply body too large")
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		readErr  error
		id       string
		rejected bool
		message  string
	}{
		{name: "accepted", status: 200, body: `{"status":"IN_QUEUE","request_id":" req-1 "}`, id: "req-1"},
		{name: "missing id", status: 200, body: `{"status":"IN_QUEUE"}`},
		{name: "blank id", status: 200, body: `{"request_id":"  "}`},
		{name: "numeric id", status: 200, body: `{"request_id":7}`},
		{name: "not json", status: 200, body: `<html>`},
		{
			name: "unprocessable", status: 422, body: `{"detail":[{"msg":"bad prompt"}]}`, rejected: true,
			message: `submit answered HTTP 422: [{"msg":"bad prompt"}]`,
		},
		{
			name: "unauthorized", status: 401, body: `{"detail":"No key"}`, rejected: true,
			message: "submit answered HTTP 401: No key",
		},
		{name: "rate limited", status: 429, body: ``, rejected: true, message: "submit answered HTTP 429"},
		{name: "server error", status: 503, body: `{"detail":"busy"}`, message: "submit answered HTTP 503"},
		{name: "read error", status: 200, readErr: readErr, message: "submit reply body too large"},
		{name: "read error on 4xx", status: 400, readErr: readErr},
		{name: "redirect", status: 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accepted, err := DecodeSubmission(tc.status, []byte(tc.body), tc.readErr)
			if tc.id != "" {
				if err != nil || accepted.RequestID != tc.id {
					t.Fatalf("%+v %v", accepted, err)
				}
				return
			}
			var submit *SubmitError
			if !errors.As(err, &submit) || submit.Rejected != tc.rejected || submit.Status != tc.status {
				t.Fatalf("%#v", err)
			}
			if tc.message != "" && submit.Error() != tc.message {
				t.Fatalf("message %q, want %q", submit.Error(), tc.message)
			}
		})
	}
}

func TestDecodeStatus(t *testing.T) {
	status, err := DecodeStatus(202, []byte(`{"status":"IN_QUEUE","queue_position":3}`), nil)
	if err != nil || status.State != "pending" || status.ProviderState != "IN_QUEUE" || status.Error != nil {
		t.Fatalf("%+v %v", status, err)
	}
	status, err = DecodeStatus(202, []byte(`{"status":"IN_PROGRESS","logs":null}`), nil)
	if err != nil || status.State != "pending" || status.ProviderState != "IN_PROGRESS" {
		t.Fatalf("%+v %v", status, err)
	}
	status, err = DecodeStatus(200, []byte(`{"status":"COMPLETED","error":null}`), nil)
	if err != nil || status.State != "completed" || status.Error != nil {
		t.Fatalf("%+v %v", status, err)
	}
	status, err = DecodeStatus(200, []byte(`{"status":"COMPLETED","error":"content policy","error_type":"x"}`), nil)
	if err != nil || status.State != "completed" || status.Error == nil || *status.Error != "content policy" {
		t.Fatalf("%+v %v", status, err)
	}
	status, err = DecodeStatus(200, []byte(`{"status":"COMPLETED","error_type":"timeout"}`), nil)
	if err != nil || status.Error == nil || *status.Error != "timeout" {
		t.Fatalf("%+v %v", status, err)
	}
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		readErr  error
		contract bool
	}{
		{"unknown status", 200, `{"status":"CANCELLED"}`, nil, true},
		{"missing status", 200, `{}`, nil, true},
		{"not found", 404, `{"detail":"Request not found"}`, nil, true},
		{"server error", 500, ``, nil, false},
		{"rate limited", 429, ``, nil, false},
		{"unauthorized", 401, ``, nil, false},
		{"read error", 200, ``, errors.New("reply body too large"), false},
		{"not json", 200, `<html>`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeStatus(tc.status, []byte(tc.body), tc.readErr)
			var query *QueryError
			if !errors.As(err, &query) || query.Contract != tc.contract {
				t.Fatalf("%#v", err)
			}
		})
	}
}

func TestDecodeVideoResult(t *testing.T) {
	success := `{"video":{"url":"https://v3b.fal.media/files/a.mp4","content_type":"video/mp4"},"seed":1}`
	result, err := DecodeVideoResult(200, []byte(success), nil)
	if err != nil || result.State != "succeeded" || result.VideoURL != "https://v3b.fal.media/files/a.mp4" ||
		result.Message != nil {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = DecodeVideoResult(422, []byte(`{"detail":"Prompt violates policy"}`), nil)
	if err != nil || result.State != "failed" || result.Message == nil || *result.Message != "Prompt violates policy" {
		t.Fatalf("%+v %v", result, err)
	}
	long := strings.Repeat("é", 400)
	result, err = DecodeVideoResult(400, []byte(long), nil)
	if err != nil || result.State != "failed" || result.Message == nil || *result.Message != strings.Repeat("é", 300) {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = DecodeVideoResult(500, nil, nil)
	var query *QueryError
	if !errors.As(err, &query) || query.Contract {
		t.Fatalf("%+v %v", result, err)
	}
	result, err = DecodeVideoResult(404, nil, nil)
	if err != nil || result.State != "failed" || result.Message == nil ||
		*result.Message != "result answered HTTP 404" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		readErr  error
		contract bool
	}{
		{"missing video", 200, `{"seed":1}`, nil, true},
		{"missing url", 200, `{"video":{}}`, nil, true},
		{"non-https url", 200, `{"video":{"url":"http://v3b.fal.media/files/a.mp4"}}`, nil, true},
		{"relative url", 200, `{"video":{"url":"/files/a.mp4"}}`, nil, true},
		{"rate limited", 429, `{"detail":"slow down"}`, nil, false},
		{"bad gateway", 502, ``, nil, false},
		{"read error", 200, ``, errors.New("reply body too large"), false},
		{"read error on 4xx", 422, ``, errors.New("reply body too large"), false},
		{"not json", 200, `<html>`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeVideoResult(tc.status, []byte(tc.body), tc.readErr)
			var query *QueryError
			if !errors.As(err, &query) || query.Contract != tc.contract {
				t.Fatalf("%#v", err)
			}
		})
	}
}
