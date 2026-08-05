package ainode

import "github.com/open-ai-sdk/ai-go/uistream"

func redactStreamError(err error) string        { return uistream.RedactStreamError(err) }
func isRedactedStreamError(message string) bool { return uistream.IsRedactedStreamError(message) }
