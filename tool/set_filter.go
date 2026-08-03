package tool

import "github.com/open-ai-sdk/ai-go/aikit"

// Restrict returns a validated set exposing only the named definitions in the
// supplied order. Invokers and the adapted executor are captured from the
// original set.
func (s *Set) Restrict(definitions []aikit.ToolDefinition) *Set {
	if s == nil {
		return nil
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		// Preserve the historical no-error signature for legacy invalid sets.
		return &Set{Definitions: cloneDefinitions(definitions), Executor: s.activeExecutor()}
	}

	restrictedDefinitions := make([]aikit.ToolDefinition, 0, len(definitions))
	restrictedInvokers := make(map[string]Invokable, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, requested := range definitions {
		if _, duplicate := seen[requested.Name]; duplicate {
			continue
		}
		definition, exists := snapshot.Lookup(requested.Name)
		if !exists {
			continue
		}
		seen[requested.Name] = struct{}{}
		restrictedDefinitions = append(restrictedDefinitions, definition)
		if invoker, ok := snapshot.Invoker(requested.Name); ok {
			restrictedInvokers[requested.Name] = invoker
		}
	}

	restricted, err := newImmutableSet(
		restrictedDefinitions,
		restrictedInvokers,
		snapshot.executor,
	)
	if err != nil {
		// The definitions were selected by name from a validated snapshot, so
		// this is unreachable unless that invariant changes in this package.
		return nil
	}
	return restricted
}

// Filter returns a set containing definitions accepted by keep.
func (s *Set) Filter(
	keep func(aikit.ToolDefinition) bool,
) *Set {
	if s == nil || keep == nil {
		return s
	}
	definitions := s.DefinitionsSnapshot()
	filtered := make([]aikit.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if keep(cloneDefinition(definition)) {
			filtered = append(filtered, definition)
		}
	}
	return s.Restrict(filtered)
}
