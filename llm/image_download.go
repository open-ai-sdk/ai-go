package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/open-ai-sdk/ai-go/transport"
)

// ImageDownloadOptions configures a download from a trusted image URL. Unlike
// ResultDownloader this helper does not enforce host or address restrictions.
// Do not pass untrusted URLs without an application-level trust policy.
type ImageDownloadOptions struct {
	Timeout    time.Duration
	Headers    map[string]string
	HTTPClient transport.Doer
}

// ImageDownloadError reports an unsuccessful HTTP status without exposing the
// response body or URL.
type ImageDownloadError struct{ StatusCode int }

func (e *ImageDownloadError) Error() string {
	return fmt.Sprintf("image download returned HTTP %d", e.StatusCode)
}

// DownloadImage downloads bytes and the response Content-Type. A zero timeout
// uses a 30-second client. A custom HTTPClient controls its own timeout.
func DownloadImage(ctx context.Context, endpoint string, options ImageDownloadOptions) ([]byte, string, error) {
	client := options.HTTPClient
	if client == nil {
		timeout := options.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	for key, value := range options.Headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp == nil || resp.Body == nil {
		return nil, "", fmt.Errorf("image download response has no body")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", &ImageDownloadError{StatusCode: resp.StatusCode}
	}
	return data, resp.Header.Get("Content-Type"), nil
}
