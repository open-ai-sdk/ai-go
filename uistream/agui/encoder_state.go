package agui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// The RFC-6902 operation set, split by which members each op requires. A patch
// is checked rather than forwarded blindly because a consumer applying a
// malformed patch cannot tell a rejected update from one that never arrived.
var (
	patchOpsNeedingValue = map[string]bool{"add": true, "replace": true, "test": true}
	patchOpsNeedingFrom  = map[string]bool{"move": true, "copy": true}
	patchOpsValueless    = map[string]bool{"remove": true}
)

func knownPatchOp(op string) bool {
	return patchOpsNeedingValue[op] || patchOpsNeedingFrom[op] || patchOpsValueless[op]
}

// isJSONPointer reports RFC-6901 syntax: the empty string, or a string of
// "/"-prefixed segments.
func isJSONPointer(path string) bool {
	return path == "" || strings.HasPrefix(path, "/")
}

// stateSnapshotEvent publishes a full state document produced by the run. It is
// distinct from stateSnapshot, which echoes the request's own state at an
// interrupt boundary and carries no run-produced information.
func (e *encoder) stateSnapshotEvent(event aikit.StepEvent) ([]uistream.Frame, error) {
	if len(event.State) == 0 {
		return nil, nil
	}
	if !json.Valid(event.State) {
		return nil, errors.New("agui: state snapshot is not valid JSON")
	}
	// Emitted as RawMessage so key order and number formatting survive the
	// round trip; re-marshalling a decoded value would reorder both.
	return e.event(eventStateSnapshot, map[string]any{"snapshot": json.RawMessage(event.State)})
}

// stateDeltaEvent publishes an RFC-6902 patch. The engine never applies it — the
// consuming application owns that, and owns applying it safely to its own
// object.
func (e *encoder) stateDeltaEvent(event aikit.StepEvent) ([]uistream.Frame, error) {
	if len(event.StatePatch) == 0 {
		return nil, nil
	}
	if err := validatePatch(event.StatePatch); err != nil {
		return nil, err
	}
	return e.event(eventStateDelta, map[string]any{"delta": json.RawMessage(event.StatePatch)})
}

func validatePatch(raw json.RawMessage) error {
	var operations []json.RawMessage
	if err := json.Unmarshal(raw, &operations); err != nil {
		return fmt.Errorf("agui: state delta must be an RFC-6902 operation array: %w", err)
	}
	// A JSON null unmarshals into a nil slice without error, so it has to be
	// rejected explicitly rather than treated as an empty patch.
	if operations == nil {
		return errors.New("agui: state delta must be an array, not null")
	}
	for index, operation := range operations {
		if err := validatePatchOperation(operation); err != nil {
			return fmt.Errorf("agui: state delta operation %d %w", index, err)
		}
	}
	return nil
}

// validatePatchOperation decodes into a member map rather than a struct so
// "value": null — a legitimate value — stays distinguishable from an absent
// "value". Unmarshalling null into a *json.RawMessage yields a nil pointer,
// which would conflate the two.
func validatePatchOperation(operation json.RawMessage) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(operation, &members); err != nil {
		return fmt.Errorf("is not an object: %w", err)
	}
	if members == nil {
		return errors.New("is null, not an object")
	}

	op, err := patchString(members, "op")
	if err != nil || !knownPatchOp(op) {
		return errors.New("has an unsupported op")
	}
	path, err := patchString(members, "path")
	if err != nil {
		return errors.New("has no usable path")
	}
	if !isJSONPointer(path) {
		return errors.New("has a path that is not a JSON pointer")
	}
	if patchOpsNeedingValue[op] {
		if _, present := members["value"]; !present {
			return errors.New("has no value")
		}
	}
	if patchOpsNeedingFrom[op] {
		from, err := patchString(members, "from")
		if err != nil {
			return errors.New("has no usable from")
		}
		if !isJSONPointer(from) {
			return errors.New("has a from that is not a JSON pointer")
		}
	}
	return nil
}

func patchString(members map[string]json.RawMessage, key string) (string, error) {
	raw, present := members[key]
	if !present {
		return "", fmt.Errorf("missing %q", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%q is not a string: %w", key, err)
	}
	return value, nil
}
