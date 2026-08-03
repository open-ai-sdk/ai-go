package aikit

import "reflect"

// Usage holds token counts and the provider's raw usage payload.
type Usage struct {
	InputTokens         int
	InputTokenDetails   InputTokenDetails
	OutputTokens        int
	OutputTokenDetails  OutputTokenDetails
	TotalTokens         int
	ToolUsePromptTokens int
	Raw                 map[string]any
}

type InputTokenDetails struct {
	NoCacheTokens    int
	CacheReadTokens  int
	CacheWriteTokens int
}

type OutputTokenDetails struct {
	TextTokens      int
	ReasoningTokens int
}

// HasValues reports whether at least one numeric usage counter was supplied.
// Raw provider metadata alone is not a token count.
func (u Usage) HasValues() bool {
	return u.InputTokens != 0 ||
		u.InputTokenDetails.NoCacheTokens != 0 ||
		u.InputTokenDetails.CacheReadTokens != 0 ||
		u.InputTokenDetails.CacheWriteTokens != 0 ||
		u.OutputTokens != 0 ||
		u.OutputTokenDetails.TextTokens != 0 ||
		u.OutputTokenDetails.ReasoningTokens != 0 ||
		u.TotalTokens != 0 ||
		u.ToolUsePromptTokens != 0
}

// Add returns the fieldwise total of two independently reported usages.
// TotalTokens is added exactly as supplied and is never inferred from details.
// Raw follows the latest non-nil snapshot policy and is independently cloned.
func (u Usage) Add(other Usage) Usage {
	raw := u.Raw
	if other.Raw != nil {
		raw = other.Raw
	}
	return Usage{
		InputTokens: u.InputTokens + other.InputTokens,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens:    u.InputTokenDetails.NoCacheTokens + other.InputTokenDetails.NoCacheTokens,
			CacheReadTokens:  u.InputTokenDetails.CacheReadTokens + other.InputTokenDetails.CacheReadTokens,
			CacheWriteTokens: u.InputTokenDetails.CacheWriteTokens + other.InputTokenDetails.CacheWriteTokens,
		},
		OutputTokens: u.OutputTokens + other.OutputTokens,
		OutputTokenDetails: OutputTokenDetails{
			TextTokens:      u.OutputTokenDetails.TextTokens + other.OutputTokenDetails.TextTokens,
			ReasoningTokens: u.OutputTokenDetails.ReasoningTokens + other.OutputTokenDetails.ReasoningTokens,
		},
		TotalTokens:         u.TotalTokens + other.TotalTokens,
		ToolUsePromptTokens: u.ToolUsePromptTokens + other.ToolUsePromptTokens,
		Raw:                 cloneUsageMap(raw),
	}
}

// Accumulate adds another independently reported usage into u.
func (u *Usage) Accumulate(other Usage) {
	*u = u.Add(other)
}

type usageCloneVisit struct {
	typeOf  reflect.Type
	pointer uintptr
	length  int
	cap     int
}

func cloneUsageMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned, ok := cloneUsageValue(
		reflect.ValueOf(values),
		make(map[usageCloneVisit]reflect.Value),
	).Interface().(map[string]any)
	if !ok {
		panic("aikit: cloned usage metadata changed type")
	}
	return cloned
}

func cloneUsageValue(value reflect.Value, seen map[usageCloneVisit]reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(cloneUsageValue(value.Elem(), seen))
		return result
	case reflect.Map, reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := usageCloneVisit{typeOf: value.Type(), pointer: uintptr(value.UnsafePointer())}
		if value.Kind() == reflect.Slice {
			visit.length, visit.cap = value.Len(), value.Cap()
		}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		if value.Kind() == reflect.Map {
			result := reflect.MakeMapWithSize(value.Type(), value.Len())
			seen[visit] = result
			iterator := value.MapRange()
			for iterator.Next() {
				result.SetMapIndex(iterator.Key(), cloneUsageValue(iterator.Value(), seen))
			}
			return result
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		seen[visit] = result
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneUsageValue(value.Index(i), seen))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneUsageValue(value.Index(i), seen))
		}
		return result
	default:
		return value
	}
}
