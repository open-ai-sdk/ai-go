package kie

import "strings"

const upstreamHost = "https://api.kie.ai"

// buildKieURL maps a logical Kie API path onto either the upstream host or a
// proxy mirror set via Config.BaseURL.
//
// Mapping (BaseURL set):
//
//	/api/v1/<rest>           -> {BaseURL}/api/kie/<rest>
//	/api/file-base64-upload  -> {BaseURL}/api/kie/file/base64-upload
//	/api/file-stream-upload  -> {BaseURL}/api/kie/file/stream-upload
//
// Mapping (BaseURL empty):
//
//	<path> -> https://api.kie.ai<path>
//
// path must start with "/".
func buildKieURL(cfg *Config, path string) string {
	if cfg.BaseURL == "" {
		return upstreamHost + path
	}

	base := strings.TrimRight(cfg.BaseURL, "/")

	switch {
	case strings.HasPrefix(path, "/api/v1/"):
		// /api/v1/jobs/createTask -> {base}/api/kie/jobs/createTask
		return base + "/api/kie/" + strings.TrimPrefix(path, "/api/v1/")

	case path == "/api/file-base64-upload":
		return base + "/api/kie/file/base64-upload"

	case path == "/api/file-stream-upload":
		return base + "/api/kie/file/stream-upload"

	default:
		// Unknown path: prepend /api/kie verbatim. Defensive — should not
		// be reached for v1 model scope.
		return base + "/api/kie" + path
	}
}
