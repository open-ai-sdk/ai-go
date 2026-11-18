package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResultDownloaderSecurity(t *testing.T) {
	for _, text := range []string{"0.1.2.3", "10.0.0.1", "127.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1", "::", "::1", "fd00::1", "fe80::1", "ff00::1", "2001:db8::1", "::ffff:10.1.2.3"} {
		if IsPublicIP(netip.MustParseAddr(text)) {
			t.Errorf("accepted nonpublic %s", text)
		}
	}
	for _, text := range []string{"8.8.8.8", "1.1.1.1", "100.63.255.255", "100.128.0.1", "198.20.0.1", "2606:4700::1111", "::ffff:8.8.8.8"} {
		if !IsPublicIP(netip.MustParseAddr(text)) {
			t.Errorf("rejected public %s", text)
		}
	}
	d := NewResultDownloader(
		ResultDownloadOptions{AllowedHosts: []string{" .CDN.example.com "}, MaxBytes: 10, Timeout: time.Second},
	)
	for _, host := range []string{"cdn.example.com", "a.cdn.example.com", "CDN.EXAMPLE.COM."} {
		if !d.AllowsHost(host) {
			t.Error(host)
		}
	}
	for _, host := range []string{"evilcdn.example.com", "cdn.example.com.evil", "example.com"} {
		if d.AllowsHost(host) {
			t.Error(host)
		}
	}
	for _, endpoint := range []string{"not a url", "http://cdn.example.com/v", "https://user:pw@cdn.example.com/v", "https://evil.example/v"} {
		_, err := d.Download(context.Background(), endpoint)
		var download *ResultDownloadError
		if !errors.As(err, &download) || !download.Refused {
			t.Errorf("%s: %v", endpoint, err)
		}
	}
	private := NewResultDownloader(
		ResultDownloadOptions{AllowedHosts: []string{"127.0.0.1"}, MaxBytes: 10, Timeout: time.Second},
	)
	_, err := private.Download(context.Background(), "https://127.0.0.1/v")
	var download *ResultDownloadError
	if !errors.As(err, &download) || !download.Refused {
		t.Fatalf("private address: %v", err)
	}
}

func TestResultDownloaderCapsRedirectsAndHeaders(t *testing.T) {
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "custom-agent" {
			t.Error("missing explicit user agent")
		}
		switch r.URL.Path {
		case "/ok":
			fmt.Fprint(w, "video")
		case "/large":
			w.Header().Set("Content-Length", "1000")
			fmt.Fprint(w, "large")
		case "/chunked":
			w.(http.Flusher).Flush()
			fmt.Fprint(w, strings.Repeat("x", 20))
		case "/redirect":
			http.Redirect(w, r, "/destination", http.StatusFound)
		case "/destination":
			redirected.Add(1)
		case "/gone":
			w.WriteHeader(410)
		case "/server":
			w.WriteHeader(503)
		case "/slow":
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/empty":
		}
	}))
	defer server.Close()
	d := NewResultDownloader(
		ResultDownloadOptions{
			AllowedHosts:      []string{"127.0.0.1"},
			MaxBytes:          10,
			Timeout:           100 * time.Millisecond,
			AllowLoopbackHTTP: true,
			Headers:           map[string]string{"User-Agent": "custom-agent"},
		},
	)
	data, err := d.Download(context.Background(), server.URL+"/ok")
	if err != nil || string(data) != "video" {
		t.Fatalf("%q %v", data, err)
	}
	for _, tc := range []struct {
		path    string
		refused bool
	}{{"/large", true}, {"/chunked", true}, {"/redirect", true}, {"/gone", true}, {"/server", false}, {"/slow", false}, {"/empty", false}} {
		_, err = d.Download(context.Background(), server.URL+tc.path)
		var download *ResultDownloadError
		if !errors.As(err, &download) || download.Refused != tc.refused {
			t.Errorf("%s: %v", tc.path, err)
		}
	}
	if redirected.Load() != 0 {
		t.Fatal("followed redirect")
	}
}
