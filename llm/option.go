package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// ProviderOption is implemented by typed provider-specific option structs.
//
// Providers should document their typed struct as the primary construction
// path. A map[string]any value is reserved for JSON-decoded input.
type ProviderOption interface {
	ProviderName() string
}

// IsNilProviderOption reports whether option is nil, including an interface
// containing a typed nil pointer. Builders treat nil options as no-ops.
func IsNilProviderOption(option ProviderOption) bool {
	if option == nil {
		return true
	}
	value := reflect.ValueOf(option)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// ProviderOptionsError reports invalid JSON-decoded options for one provider.
type ProviderOptionsError struct {
	Provider string
	Err      error
}

func (err *ProviderOptionsError) Error() string {
	return fmt.Sprintf("llm: invalid %s provider options: %v", err.Provider, err.Err)
}

// Unwrap returns the underlying decoding error.
func (err *ProviderOptionsError) Unwrap() error {
	return err.Err
}

// DecodeJSONProviderOptions strictly decodes a map produced by encoding/json.
// Unknown fields and wrong field types are returned to the caller.
func DecodeJSONProviderOptions(provider string, value map[string]any, destination any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return &ProviderOptionsError{Provider: provider, Err: err}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return &ProviderOptionsError{Provider: provider, Err: err}
	}
	return nil
}

// ProviderOptionTypeError reports a non-map, non-typed provider-options value.
func ProviderOptionTypeError(provider string, value any) error {
	return &ProviderOptionsError{
		Provider: provider,
		Err:      fmt.Errorf("got %T; want the provider's typed options or map[string]any", value),
	}
}
