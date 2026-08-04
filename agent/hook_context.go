package agent

import "sync"

// scratchpad is deliberately run-local. Its mutex is never held while a hook
// callback executes; Update is the sole explicitly atomic callback operation.
type scratchpad struct {
	mu     sync.RWMutex
	values map[any]any
}

func newScratchpad() *scratchpad { return &scratchpad{values: make(map[any]any)} }

// Load returns the value stored under key for this run.
func (h HookContext) Load(key any) (any, bool) {
	if h.scratchpad == nil {
		return nil, false
	}
	h.scratchpad.mu.RLock()
	defer h.scratchpad.mu.RUnlock()
	value, ok := h.scratchpad.values[key]
	return value, ok
}

// Store stores a run-local value. Keys must be comparable.
func (h HookContext) Store(key, value any) {
	if h.scratchpad == nil {
		return
	}
	h.scratchpad.mu.Lock()
	defer h.scratchpad.mu.Unlock()
	h.scratchpad.values[key] = value
}

// Delete removes a run-local value.
func (h HookContext) Delete(key any) {
	if h.scratchpad == nil {
		return
	}
	h.scratchpad.mu.Lock()
	defer h.scratchpad.mu.Unlock()
	delete(h.scratchpad.values, key)
}

// Update atomically replaces the value at key. The callback must not block or
// re-enter this scratchpad because it runs while the scratchpad is locked.
func (h HookContext) Update(key any, fn func(any, bool) (any, bool)) {
	if h.scratchpad == nil || fn == nil {
		return
	}
	h.scratchpad.mu.Lock()
	defer h.scratchpad.mu.Unlock()
	value, ok := h.scratchpad.values[key]
	replacement, keep := fn(value, ok)
	if keep {
		h.scratchpad.values[key] = replacement
	} else {
		delete(h.scratchpad.values, key)
	}
}

// ScratchpadGet returns a typed run-local value.
func ScratchpadGet[T any](h HookContext, key any) (T, bool) {
	var zero T
	value, ok := h.Load(key)
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	return typed, ok
}

// ScratchpadUpdate atomically updates a typed run-local value.
func ScratchpadUpdate[T any](h HookContext, key any, fn func(T, bool) (T, bool)) {
	if fn == nil {
		return
	}
	h.Update(key, func(value any, present bool) (any, bool) {
		typed, ok := value.(T)
		if !present {
			ok = false
		}
		next, keep := fn(typed, ok)
		return next, keep
	})
}
