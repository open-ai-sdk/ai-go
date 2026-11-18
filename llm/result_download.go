package llm

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// ResultDownloadOptions controls a pinned, allowlisted result download.
type ResultDownloadOptions struct {
	AllowedHosts []string
	MaxBytes     int64
	Timeout      time.Duration
	// AllowLoopbackHTTP permits only literal 127.0.0.1 over HTTP for local endpoints.
	AllowLoopbackHTTP bool
	Headers           map[string]string
}

// ResultDownloadError distinguishes a refused URL/result from a transient failure.
type ResultDownloadError struct {
	Refused bool
	Message string
}

func (e *ResultDownloadError) Error() string {
	if e.Refused {
		return "result download refused: " + e.Message
	}
	return "result download failed: " + e.Message
}

// ResultDownloader resolves once, rejects every non-public answer, then pins
// its dialer to those addresses. No proxy or redirect can bypass that check.
type ResultDownloader struct {
	allowedHosts      []string
	maxBytes          int64
	timeout           time.Duration
	allowLoopbackHTTP bool
	headers           map[string]string
}

func NewResultDownloader(options ResultDownloadOptions) *ResultDownloader {
	hosts, maxBytes, timeout := options.AllowedHosts, options.MaxBytes, options.Timeout
	allowed := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimLeft(strings.TrimSpace(host), "."))
		if host != "" {
			allowed = append(allowed, host)
		}
	}
	headers := make(map[string]string, len(options.Headers))
	for key, value := range options.Headers {
		headers[key] = value
	}
	return &ResultDownloader{
		allowedHosts:      allowed,
		maxBytes:          maxBytes,
		timeout:           timeout,
		allowLoopbackHTTP: options.AllowLoopbackHTTP,
		headers:           headers,
	}
}

func (d *ResultDownloader) AllowedHosts() []string { return append([]string(nil), d.allowedHosts...) }

func (d *ResultDownloader) AllowsHost(host string) bool {
	host = strings.ToLower(strings.TrimRight(host, "."))
	for _, allowed := range d.allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func (d *ResultDownloader) Download(ctx context.Context, endpoint string) ([]byte, error) {
	u, loopback, err := d.resultURL(endpoint)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	refused := func(message string) ([]byte, error) { return nil, &ResultDownloadError{true, message} }
	transient := func(message string) ([]byte, error) { return nil, &ResultDownloadError{false, message} }
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return transient("DNS lookup failed: " + err.Error())
	}
	if len(addresses) == 0 {
		return transient("the host resolved to nothing")
	}
	for _, ip := range addresses {
		if !loopback && !IsPublicIP(ip) {
			return refused("the result host resolves to a non-public address")
		}
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	transport := &http.Transport{DialContext: func(dialCtx context.Context, network, _ string) (net.Conn, error) {
		var last error
		for _, ip := range addresses {
			conn, dialErr := (&net.Dialer{}).DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			last = dialErr
		}
		return nil, last
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return transient("http client: " + err.Error())
	}
	for key, value := range d.headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return transient("request failed: " + err.Error())
	}
	defer resp.Body.Close()
	return readResultBody(resp, d.maxBytes)
}

func (d *ResultDownloader) resultURL(endpoint string) (*url.URL, bool, error) {
	refused := func(message string) (*url.URL, bool, error) {
		return nil, false, &ResultDownloadError{true, message}
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return refused("not a URL")
	}
	host := u.Hostname()
	if host == "" {
		return refused("the URL has no host")
	}
	loopback := d.allowLoopbackHTTP && u.Scheme == "http" && host == "127.0.0.1"
	if u.Scheme != "https" && !loopback {
		return refused("only https result URLs are fetched")
	}
	if u.User != nil {
		return refused("credentials in the URL")
	}
	if !d.AllowsHost(host) {
		return refused(fmt.Sprintf("host %s is not an allowlisted result host", host))
	}
	if d.maxBytes <= 0 {
		return refused("the download cap is zero")
	}
	return u, loopback, nil
}

func readResultBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	refused := func(message string) ([]byte, error) { return nil, &ResultDownloadError{true, message} }
	transient := func(message string) ([]byte, error) { return nil, &ResultDownloadError{false, message} }
	status := resp.StatusCode
	if status >= 300 && status < 400 {
		return refused(fmt.Sprintf("the result URL redirects (%d)", status))
	}
	switch status {
	case 401, 403, 404, 410:
		return refused(fmt.Sprintf("the result URL answered %d (expired or not ours)", status))
	}
	if status < 200 || status >= 300 {
		return transient(fmt.Sprintf("the result host answered %d", status))
	}
	if resp.ContentLength > maxBytes {
		return refused(fmt.Sprintf("the result is larger than %d bytes", maxBytes))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return transient("body: " + err.Error())
	}
	if int64(len(data)) > maxBytes {
		return refused(fmt.Sprintf("the result is larger than %d bytes", maxBytes))
	}
	if len(data) == 0 {
		return transient("the result body is empty")
	}
	return data, nil
}

func IsPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	if ip.Is4() {
		return isPublicIPv4(ip)
	}
	return isPublicIPv6(ip)
}

func isPublicIPv4(ip netip.Addr) bool {
	b := ip.As4()
	a, c := b[0], b[1]
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.IsUnspecified() || ip.IsMulticast() || a == 0 || a >= 240 || isDocumentationIPv4(b) {
		return false
	}
	shared := a == 100 && c >= 64 && c < 128
	protocol := a == 192 && c == 0
	benchmark := a == 198 && c >= 18 && c < 20
	return !shared && !protocol && !benchmark
}

func isDocumentationIPv4(b [4]byte) bool {
	return (b[0] == 198 && b[1] == 51 && b[2] == 100) ||
		(b[0] == 203 && b[1] == 0 && b[2] == 113)
}

func isPublicIPv6(ip netip.Addr) bool {
	b := ip.As16()
	first := uint16(b[0])<<8 | uint16(b[1])
	return !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsMulticast() &&
		first&0xfe00 != 0xfc00 && first&0xffc0 != 0xfe80 &&
		(first != 0x2001 || b[2] != 0x0d || b[3] != 0xb8)
}
