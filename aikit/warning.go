package aikit

// Warning is a non-fatal advisory from a provider.
type Warning struct {
	Type    string
	Message string
	Setting string
}
