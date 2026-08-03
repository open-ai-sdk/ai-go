package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/llm"
)

func TestNewClientValidatesConfigurationEagerly(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(Config{}); err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("missing API key error = %v", err)
	}
	if _, err := NewClient(Config{APIKey: "test-key", BaseURL: "://bad"}); err == nil {
		t.Fatal("invalid base URL unexpectedly succeeded")
	}
	if _, err := NewClient(Config{APIKey: "test-key", Timeout: -1}); err == nil {
		t.Fatal("negative timeout unexpectedly succeeded")
	}
}

func TestClientCreatesCapabilitySpecificModelHandles(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	first := client.CompletionModel("gpt-5")
	second := client.CompletionModel("gpt-4.1")
	chat := client.ChatModel("gpt-4o")

	var _ llm.Model = first
	var _ llm.Model = chat
	if first.ModelID() != "gpt-5" || second.ModelID() != "gpt-4.1" || chat.ModelID() != "gpt-4o" {
		t.Fatalf("unexpected model IDs: %q %q %q", first.ModelID(), second.ModelID(), chat.ModelID())
	}
	if first.client != client || second.client != client {
		t.Fatal("completion handles do not share their owning Client")
	}
}

func TestClientFormattingDoesNotLeakAPIKey(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{
		APIKey:  "super-secret-value",
		BaseURL: "https://user:password@example.com/v1?token=query-secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for _, formatted := range []string{
		fmt.Sprintf("%v", client), fmt.Sprintf("%#v", client),
		fmt.Sprintf("%+v", *client), fmt.Sprintf("%#v", *client),
	} {
		if strings.Contains(formatted, "super-secret-value") || strings.Contains(formatted, "password") ||
			strings.Contains(formatted, "query-secret") {
			t.Fatalf("formatted client leaked credentials: %s", formatted)
		}
	}
}

func TestClientUploadFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/files" {
			t.Errorf("path = %q", req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := req.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"file-123","object":"file","filename":"notes.txt","bytes":5,"status":"processed"}`,
		)
	}))
	defer server.Close()

	client, err := NewClient(Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	file, err := client.UploadFile(context.Background(), UploadFileRequest{
		Filename: "notes.txt",
		Purpose:  FilePurposeUserData,
		Data:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if file.ID != "file-123" || file.Filename != "notes.txt" {
		t.Fatalf("file = %#v", file)
	}
}

func TestLegacyLanguageModelStillDefersInvalidTransportError(t *testing.T) {
	t.Parallel()

	model := NewLanguageModel("gpt-5", Config{BaseURL: "://bad"})
	if model == nil || model.clientErr == nil {
		t.Fatalf("legacy model did not retain deferred configuration error: %#v", model)
	}
}

func TestLegacyLanguageModelRetainsNegativeTimeoutBehavior(t *testing.T) {
	t.Parallel()

	model := NewLanguageModel("gpt-5", Config{
		APIKey:       "test-key",
		Timeout:      -1,
		ChunkTimeout: -1,
	})
	if model.clientErr != nil || model.client == nil {
		t.Fatalf("legacy timeout configuration was rejected: %v", model.clientErr)
	}
}
