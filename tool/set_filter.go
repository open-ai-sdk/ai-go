package tool

import "github.com/open-ai-sdk/ai-go/aikit"

// Restrict returns a set exposing only definitions present in definitions.
// Invokers and the adapted executor are retained from the original set.
func (s *Set) Restrict(definitions []aikit.ToolDefinition) *Set {
	if s == nil {
		return nil
	}
	restricted := &Set{
		Definitions: make([]aikit.ToolDefinition, 0, len(definitions)),
		Executor:    s.Executor,
		index:       make(map[string]int, len(definitions)),
		invokers:    make(map[string]Invokable, len(definitions)),
	}
	for _, definition := range definitions {
		if _, exists := restricted.index[definition.Name]; exists {
			continue
		}
		restricted.index[definition.Name] = len(restricted.Definitions)
		restricted.Definitions = append(
			restricted.Definitions,
			cloneDefinition(definition),
		)
		if invoker, ok := s.invokers[definition.Name]; ok {
			restricted.invokers[definition.Name] = invoker
		}
	}
	restricted.indexOnce.Do(func() {})
	return restricted
}

// Filter returns a set containing definitions accepted by keep.
func (s *Set) Filter(
	keep func(aikit.ToolDefinition) bool,
) *Set {
	if s == nil || keep == nil {
		return s
	}
	definitions := make([]aikit.ToolDefinition, 0, len(s.Definitions))
	for _, definition := range s.Definitions {
		if keep(cloneDefinition(definition)) {
			definitions = append(definitions, definition)
		}
	}
	return s.Restrict(definitions)
}
