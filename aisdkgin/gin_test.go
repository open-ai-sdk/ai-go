package aisdkgin

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandlerRoutesThroughNetHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/chat", Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-accel-buffering", "no")
		_, _ = w.Write([]byte("streamed"))
	})))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/chat", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("x-accel-buffering"); got != "no" {
		t.Fatalf("x-accel-buffering = %q, want no", got)
	}
	if recorder.Body.String() != "streamed" {
		t.Fatalf("unexpected body %q", recorder.Body.String())
	}
}

func TestHandlerPreservesDisconnectCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stopped := make(chan struct{})
	router := gin.New()
	router.POST("/chat", Handler(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer close(stopped)
		_, _ = w.Write([]byte("data: started\n\n"))
		w.(http.Flusher).Flush()
		<-request.Context().Done()
	})))

	server := httptest.NewServer(router)
	defer server.Close()
	response, err := server.Client().Post(server.URL+"/chat", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("gin request context was not cancelled after client disconnect")
	}
}
