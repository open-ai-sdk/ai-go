package uistream

// Options configure one Pipe invocation.
type Options struct {
	MessageID    string
	OnWriteError func(error)
	Extra        map[string]any
}
