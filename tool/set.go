package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Set is a collection of tool definitions and invokers.
//
// Construct sets with NewSet or NewSetFromExecutor. The exported legacy fields
// remain available while the ai facade is being restructured; constructor-made
// sets eagerly build a duplicate-safe O(1) registry.
type Set struct {
	Definitions []aikit.ToolDefinition
	Executor    Executor

	indexOnce sync.Once
	index     map[string]int
	invokers  map[string]Invokable
	indexErr  error
}

// NewSet registers invokable tools and rejects duplicate names.
func NewSet(tools ...Invokable) (*Set, error) {
	set := &Set{
		Definitions: make([]aikit.ToolDefinition, 0, len(tools)),
		index:       make(map[string]int, len(tools)),
		invokers:    make(map[string]Invokable, len(tools)),
	}
	for _, candidate := range tools {
		if candidate == nil || isNilInvokable(candidate) {
			return nil, errors.New("tool: cannot register a nil tool")
		}
		definition := candidate.Describe()
		if _, exists := set.index[definition.Name]; exists {
			return nil, fmt.Errorf("tool: duplicate registration %q", definition.Name)
		}
		set.index[definition.Name] = len(set.Definitions)
		set.invokers[definition.Name] = candidate
		set.Definitions = append(set.Definitions, cloneDefinition(definition))
	}
	set.indexOnce.Do(func() {})
	return set, nil
}

// NewSetFromExecutor adapts a named string executor into a duplicate-safe Set.
func NewSetFromExecutor(
	definitions []aikit.ToolDefinition,
	executor Executor,
) (*Set, error) {
	set := &Set{
		Definitions: make([]aikit.ToolDefinition, len(definitions)),
		Executor:    executor,
		index:       make(map[string]int, len(definitions)),
	}
	for i, definition := range definitions {
		if _, exists := set.index[definition.Name]; exists {
			return nil, fmt.Errorf("tool: duplicate registration %q", definition.Name)
		}
		set.index[definition.Name] = i
		set.Definitions[i] = cloneDefinition(definition)
	}
	set.indexOnce.Do(func() {})
	return set, nil
}

// Validate reports duplicate names in sets created through legacy struct
// literals.
func (s *Set) Validate() error {
	if s == nil {
		return nil
	}
	s.indexOnce.Do(s.buildIndex)
	return s.indexErr
}

// Lookup returns the definition named name in O(1) after construction.
func (s *Set) Lookup(name string) (aikit.ToolDefinition, bool) {
	if s == nil {
		return aikit.ToolDefinition{}, false
	}
	s.indexOnce.Do(s.buildIndex)
	if s.indexErr != nil {
		return aikit.ToolDefinition{}, false
	}
	if len(s.index) != len(s.Definitions) {
		return s.scanForName(name)
	}
	i, ok := s.index[name]
	if !ok {
		return aikit.ToolDefinition{}, false
	}
	return cloneDefinition(s.Definitions[i]), true
}

// Invoke dispatches name through the typed registry or adapted Executor.
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
	if err := s.Validate(); err != nil {
		return nil, &ExecutionError{ToolName: name, Cause: err}
	}
	if invoker, ok := s.invokers[name]; ok {
		return invoker.Invoke(ctx, input)
	}
	registered := false
	if len(s.Definitions) > 0 {
		if _, ok := s.Lookup(name); !ok {
			return nil, &NoSuchToolError{
				ToolName:       name,
				AvailableTools: s.names(),
			}
		}
		registered = true
	}
	if s.Executor != nil {
		output, err := s.Executor.Execute(ctx, name, string(input))
		if err != nil {
			// Executor is the compatibility seam for existing agent
			// integrations. Preserve its error value and presentation exactly;
			// New and NewDynamic provide typed classification for new tools.
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

// Execute implements Executor without changing the raw-string output contract.
func (s *Set) Execute(
	ctx context.Context,
	name, argsJSON string,
) (string, error) {
	output, err := s.Invoke(ctx, name, json.RawMessage(argsJSON))
	return string(output), err
}

func (s *Set) buildIndex() {
	s.index = make(map[string]int, len(s.Definitions))
	for i, definition := range s.Definitions {
		if _, exists := s.index[definition.Name]; exists {
			s.indexErr = fmt.Errorf(
				"tool: duplicate registration %q",
				definition.Name,
			)
			return
		}
		s.index[definition.Name] = i
	}
}

func (s *Set) scanForName(name string) (aikit.ToolDefinition, bool) {
	for _, definition := range s.Definitions {
		if definition.Name == name {
			return cloneDefinition(definition), true
		}
	}
	return aikit.ToolDefinition{}, false
}

func (s *Set) names() []string {
	names := make([]string, 0, len(s.Definitions))
	for _, definition := range s.Definitions {
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
