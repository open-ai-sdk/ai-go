package aisdkhttp

import "net/http"

const (
	invalidRequestMessage = "invalid request body"
	streamErrorMessage    = "stream error"
)

func writeHTTPError(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}
