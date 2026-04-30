package kie

import (
	"strings"
	"testing"
)

func TestParseKieError_WithEnvelope(t *testing.T) {
	body := []byte(`{"code":401,"msg":"unauthorized"}`)
	e := parseKieError(401, body)
	if e.Code != 401 || e.Msg != "unauthorized" || e.Status != 401 {
		t.Errorf("got %+v", e)
	}
	if !strings.Contains(e.Error(), "code=401") || !strings.Contains(e.Error(), "msg=unauthorized") {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestParseKieError_NotJSON(t *testing.T) {
	e := parseKieError(503, []byte("Service Unavailable"))
	if e.Code != 0 || e.Status != 503 || e.RawBody != "Service Unavailable" {
		t.Errorf("got %+v", e)
	}
	if !strings.Contains(e.Error(), "code=0") {
		t.Errorf("Error() = %q", e.Error())
	}
}
