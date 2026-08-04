package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Set is an immutable ordered registry of tool definitions and their exact
// invokers. Construct sets with NewSet.
type Set struct {
	definitions []aikit.ToolDefinition
	index       map[string]int
	invokers    map[string]Invokable
}

// Snapshot is an immutable, run-scoped view of a Set. It keeps definitions in
// registration order and binds them to the exact invokers captured at snapshot
// time.
type Snapshot struct {
	definitions []aikit.ToolDefinition
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
	return newImmutableSet(definitions, invokers)
}

func newImmutableSet(
	definitions []aikit.ToolDefinition,
	invokers map[string]Invokable,
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
		definitions: cloned,
		index:       index,
		invokers:    boundInvokers,
	}, nil
}

// Validate reports whether the registry is valid. Constructor-made sets are
// validated eagerly, so an existing Set is always valid.
func (s *Set) Validate() error {
	return nil
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
	return len(s.definitions)
}

// Lookup returns an independently owned copy of the named definition.
func (s *Set) Lookup(name string) (aikit.ToolDefinition, bool) {
	if s == nil {
		return aikit.ToolDefinition{}, false
	}
	i, ok := s.index[name]
	if !ok {
		return aikit.ToolDefinition{}, false
	}
	return cloneDefinition(s.definitions[i]), true
}

// Invoker returns the exact typed invoker registered for name.
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
			Cause:    errors.New("nil tool set"),
		}
	}
	if invoker, ok := s.invokers[name]; ok {
		return invoker.Invoke(ctx, input)
	}
	return nil, &NoSuchToolError{ToolName: name, AvailableTools: Snapshot{definitions: s.definitions}.names()}
}

// InvokeResult dispatches through the rich capability when supplied, adapting
// legacy invokers at the released raw-JSON boundary otherwise.
func (s *Set) InvokeResult(ctx context.Context, name string, input json.RawMessage) (ExecutionResult, error) {
	if s == nil {
		return ExecutionResult{}, &ExecutionError{ToolName: name, Cause: errors.New("nil tool set")}
	}
	return Snapshot{definitions: s.definitions, index: s.index, invokers: s.invokers}.InvokeResult(ctx, name, input)
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
	return nil, &NoSuchToolError{
		ToolName:       name,
		AvailableTools: s.names(),
	}
}

// InvokeResult invokes the exact captured rich or legacy path.
func (s Snapshot) InvokeResult(ctx context.Context, name string, input json.RawMessage) (ExecutionResult, error) {
	invoker, ok := s.invokers[name]
	if !ok {
		return ExecutionResult{}, &NoSuchToolError{ToolName: name, AvailableTools: s.names()}
	}
	if rich, ok := invoker.(ResultInvokable); ok {
		return rich.InvokeResult(ctx, input)
	}
	raw, err := invoker.Invoke(ctx, input)
	if err != nil {
		return ExecutionResult{}, err
	}
	return ResultFromLegacy(raw), nil
}

func (s *Set) activeDefinitions() []aikit.ToolDefinition {
	return cloneDefinitions(s.definitions)
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
