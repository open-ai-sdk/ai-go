package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Set is an ordered registry of tool definitions and their execution paths.
//
// New code should construct sets with NewSet or NewSetFromExecutor and read
// them through Snapshot, DefinitionsSnapshot, Lookup, or Invoker. Definitions
// and Executor are retained temporarily for source compatibility with the
// legacy agent runtime; constructor-made sets never use those mutable fields
// as their internal source of truth.
type Set struct {
	// Deprecated: construct a Set with NewSet or NewSetFromExecutor and use
	// DefinitionsSnapshot to read its definitions.
	Definitions []aikit.ToolDefinition
	// Deprecated: construct a Set with NewSetFromExecutor.
	Executor Executor

	immutable   bool
	definitions []aikit.ToolDefinition
	executor    Executor
	index       map[string]int
	invokers    map[string]Invokable
}

// Snapshot is an immutable, run-scoped view of a Set. It keeps definitions in
// registration order and binds them to the exact invokers/executor captured at
// snapshot time.
type Snapshot struct {
	definitions []aikit.ToolDefinition
	executor    Executor
	index       map[string]int
	invokers    map[string]Invokable
}

// NewSet registers invokable tools in argument order.
func NewSet(tools ...Invokable) (*Set, error) {
	definitions := make([]aikit.ToolDefinition, 0, len(tools))
	invokers := make(map[string]Invokable, len(tools))
	for _, candidate := range tools {
		if candidate == nil || isNilInvokable(candidate) {
			return nil, errors.New("tool: cannot register a nil tool")
		}
		definition := candidate.Describe()
		if definition.Name == "" {
			return nil, errors.New("tool: cannot register a tool with an empty name")
		}
		if _, exists := invokers[definition.Name]; exists {
			return nil, fmt.Errorf("tool: duplicate registration %q", definition.Name)
		}
		definitions = append(definitions, definition)
		invokers[definition.Name] = candidate
	}
	return newImmutableSet(definitions, invokers, nil)
}

// NewSetFromExecutor adapts a named string executor into an ordered Set.
func NewSetFromExecutor(
	definitions []aikit.ToolDefinition,
	executor Executor,
) (*Set, error) {
	return newImmutableSet(definitions, nil, executor)
}

func newImmutableSet(
	definitions []aikit.ToolDefinition,
	invokers map[string]Invokable,
	executor Executor,
) (*Set, error) {
	cloned := cloneDefinitions(definitions)
	index, err := indexDefinitions(cloned)
	if err != nil {
		return nil, err
	}
	boundInvokers := make(map[string]Invokable, len(invokers))
	for name, invoker := range invokers {
		boundInvokers[name] = invoker
	}
	return &Set{
		// Keep independent compatibility mirrors so callers mutating the
		// legacy fields cannot mutate the canonical registry.
		Definitions: cloneDefinitions(cloned),
		Executor:    executor,
		immutable:   true,
		definitions: cloned,
		executor:    executor,
		index:       index,
		invokers:    boundInvokers,
	}, nil
}

// Validate reports invalid or duplicate definitions. Constructor-made sets
// are validated eagerly; legacy struct literals are validated on every call.
func (s *Set) Validate() error {
	if s == nil {
		return nil
	}
	if s.immutable {
		return nil
	}
	_, err := indexDefinitions(s.Definitions)
	return err
}

// Snapshot captures definitions and their exact execution paths together.
// Mutating the source definitions or the returned definition copies cannot
// change the snapshot.
func (s *Set) Snapshot() (Snapshot, error) {
	if s == nil {
		return Snapshot{}, nil
	}
	definitions := s.activeDefinitions()
	index, err := indexDefinitions(definitions)
	if err != nil {
		return Snapshot{}, err
	}
	invokers := make(map[string]Invokable, len(s.invokers))
	for name, invoker := range s.invokers {
		invokers[name] = invoker
	}
	return Snapshot{
		definitions: definitions,
		executor:    s.activeExecutor(),
		index:       index,
		invokers:    invokers,
	}, nil
}

// Clone returns an independently owned immutable Set preserving registration
// order and the exact captured execution paths. It returns nil for a nil or
// invalid legacy Set; constructor-made sets are always valid.
func (s *Set) Clone() *Set {
	if s == nil {
		return nil
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		return nil
	}
	cloned, err := newImmutableSet(
		snapshot.definitions,
		snapshot.invokers,
		snapshot.executor,
	)
	if err != nil {
		return nil
	}
	return cloned
}

// DefinitionsSnapshot returns an independently owned definition slice in
// deterministic registration order.
func (s *Set) DefinitionsSnapshot() []aikit.ToolDefinition {
	if s == nil {
		return nil
	}
	return s.activeDefinitions()
}

// Len returns the number of registered definitions.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	if s.immutable {
		return len(s.definitions)
	}
	return len(s.Definitions)
}

// Lookup returns an independently owned copy of the named definition.
func (s *Set) Lookup(name string) (aikit.ToolDefinition, bool) {
	if s == nil {
		return aikit.ToolDefinition{}, false
	}
	if s.immutable {
		i, ok := s.index[name]
		if !ok {
			return aikit.ToolDefinition{}, false
		}
		return cloneDefinition(s.definitions[i]), true
	}
	for _, definition := range s.Definitions {
		if definition.Name == name {
			return cloneDefinition(definition), true
		}
	}
	return aikit.ToolDefinition{}, false
}

// Invoker returns the exact typed invoker registered for name. Executor-backed
// sets intentionally return false because their execution path is the captured
// Executor rather than an Invokable.
func (s *Set) Invoker(name string) (Invokable, bool) {
	if s == nil {
		return nil, false
	}
	invoker, ok := s.invokers[name]
	return invoker, ok
}

// Invoke dispatches name through the registry's captured execution path.
func (s *Set) Invoke(
	ctx context.Context,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if s == nil {
		return nil, &ExecutionError{
			ToolName: name,
			Cause:    errors.New("no executor"),
		}
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		return nil, &ExecutionError{ToolName: name, Cause: err}
	}
	return snapshot.Invoke(ctx, name, input)
}

// Execute implements Executor without changing the raw-string output contract.
func (s *Set) Execute(
	ctx context.Context,
	name, argsJSON string,
) (string, error) {
	output, err := s.Invoke(ctx, name, json.RawMessage(argsJSON))
	return string(output), err
}

// Definitions returns independent copies of the snapshot's definitions.
func (s Snapshot) Definitions() []aikit.ToolDefinition {
	return cloneDefinitions(s.definitions)
}

// Len returns the number of definitions in the snapshot.
func (s Snapshot) Len() int { return len(s.definitions) }

// Lookup returns an independently owned copy of the named definition.
func (s Snapshot) Lookup(name string) (aikit.ToolDefinition, bool) {
	i, ok := s.index[name]
	if !ok {
		return aikit.ToolDefinition{}, false
	}
	return cloneDefinition(s.definitions[i]), true
}

// Invoker returns the exact typed invoker captured for name.
func (s Snapshot) Invoker(name string) (Invokable, bool) {
	invoker, ok := s.invokers[name]
	return invoker, ok
}

// Invoke dispatches through the exact execution path captured by the snapshot.
func (s Snapshot) Invoke(
	ctx context.Context,
	name string,
	input json.RawMessage,
) (json.RawMessage, error) {
	if invoker, ok := s.invokers[name]; ok {
		return invoker.Invoke(ctx, input)
	}
	registered := false
	if len(s.definitions) > 0 {
		if _, ok := s.index[name]; !ok {
			return nil, &NoSuchToolError{
				ToolName:       name,
				AvailableTools: s.names(),
			}
		}
		registered = true
	}
	if s.executor != nil {
		output, err := s.executor.Execute(ctx, name, string(input))
		if err != nil {
			// Executor is the compatibility seam for existing integrations.
			// Preserve its error value and presentation exactly.
			return nil, err
		}
		return json.RawMessage(output), nil
	}
	if registered {
		return nil, &ExecutionError{
			ToolName: name,
			Cause:    errors.New("no executor"),
		}
	}
	return nil, &NoSuchToolError{
		ToolName:       name,
		AvailableTools: s.names(),
	}
}

func (s *Set) activeDefinitions() []aikit.ToolDefinition {
	if s.immutable {
		return cloneDefinitions(s.definitions)
	}
	return cloneDefinitions(s.Definitions)
}

func (s *Set) activeExecutor() Executor {
	if s.immutable {
		return s.executor
	}
	return s.Executor
}

func indexDefinitions(definitions []aikit.ToolDefinition) (map[string]int, error) {
	index := make(map[string]int, len(definitions))
	for i, definition := range definitions {
		if definition.Name == "" {
			return nil, errors.New("tool: cannot register a tool with an empty name")
		}
		if _, exists := index[definition.Name]; exists {
			return nil, fmt.Errorf("tool: duplicate registration %q", definition.Name)
		}
		index[definition.Name] = i
	}
	return index, nil
}

func cloneDefinitions(definitions []aikit.ToolDefinition) []aikit.ToolDefinition {
	if definitions == nil {
		return nil
	}
	cloned := make([]aikit.ToolDefinition, len(definitions))
	for i, definition := range definitions {
		cloned[i] = cloneDefinition(definition)
	}
	return cloned
}

func (s Snapshot) names() []string {
	names := make([]string, 0, len(s.definitions))
	for _, definition := range s.definitions {
		names = append(names, definition.Name)
	}
	return names
}

var _ Executor = (*Set)(nil)

func isNilInvokable(candidate Invokable) bool {
	value := reflect.ValueOf(candidate)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
