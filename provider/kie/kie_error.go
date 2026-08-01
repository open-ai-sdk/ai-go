package kie

import (
	"encoding/json"
	"fmt"
)

// KieError is the structured envelope for any non-200 response that still
// carries a `{code, msg}` JSON body.
type KieError struct {
	Code    int
	Msg     string
	Status  int    // HTTP status code
	RawBody string // raw response body (for debugging)
}

// Error implements the error interface.
func (e *KieError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("kie: code=%d http=%d", e.Code, e.Status)
	}
	return fmt.Sprintf("kie: code=%d http=%d msg=%s", e.Code, e.Status, e.Msg)
}

// parseKieError extracts a KieError from a Kie response body. status is the
// HTTP status; body is the raw response. If body cannot be parsed as the
// `{code, msg}` envelope, the returned KieError still carries Status + RawBody.
func parseKieError(status int, body []byte) *KieError {
	e := &KieError{Status: status, RawBody: string(body)}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		e.Code = env.Code
		e.Msg = env.Msg
	}
	return e
}
