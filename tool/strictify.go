package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Limits documented for OpenAI strict structured outputs.
const (
	strictMaxDepth      = 10
	strictMaxEnumValues = 1000
	strictMaxProperties = 5000
)

// strictRefMaxDepth bounds $ref inlining so a self-referencing definition
// fails with an error instead of expanding forever.
const strictRefMaxDepth = 64

// StrictViolation names one place where a schema is not accepted by OpenAI
// strict mode.
type StrictViolation struct {
	// Path is a JSON-path-like location: "$" for the root, ".name" for a
	// property, "[]" for array items and ".anyOf[i]" for a union branch.
	Path string
	// Reason says why strict mode refuses the node.
	Reason string
}

// String renders the violation as "path: reason".
func (v StrictViolation) String() string { return v.Path + ": " + v.Reason }

// StrictifyError is returned by [Strictify] when the rewritten schema still
// breaks a strict-mode rule that no rewrite can fix.
type StrictifyError struct {
	Violations []StrictViolation
}

// Error lists every violation.
func (e *StrictifyError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, violation := range e.Violations {
		parts[i] = violation.String()
	}
	return "tool: schema cannot be made strict: " + strings.Join(parts, "; ")
}

// StrictifyOptions controls optional wire-preserving schema rewrites.
type StrictifyOptions struct {
	// PreserveRequiredOrder retains an explicit required array when it already
	// contains every property exactly once. Other arrays are replaced by a
	// complete sorted list. The zero value keeps Strictify's established order.
	PreserveRequiredOrder bool
	// AssumeAllPropertiesRequired skips making omitted properties nullable.
	// Every property is still added to required; existing nullability is kept.
	// The zero value preserves optionality by making omitted properties nullable.
	AssumeAllPropertiesRequired bool
}

// Strictify rewrites a JSON Schema into the shape OpenAI strict function
// calling and structured outputs accept, and fails with a [*StrictifyError]
// when a rule is left that no rewrite can satisfy (for example a root that is
// not an object, or an object with no declared properties).
//
// The rewrite, applied to every sub-schema:
//   - properties missing from "required" become nullable (anyOf with null) and
//     are then listed in "required"; already-nullable properties are kept;
//   - every object gets "additionalProperties": false and a complete
//     "required" list;
//   - local "#/$defs/X" and "#/definitions/X" references are inlined, and
//     "$defs", "definitions", "$schema" and "title" are dropped;
//   - array-valued "type" becomes "anyOf", moving type-specific keywords
//     (items, properties, required, additionalProperties) into their branch;
//   - "oneOf" of enum/const branches collapses to one enum, any other "oneOf"
//     becomes "anyOf"; "const" becomes a one-value "enum";
//   - "default" and validation keywords (format, minimum, maximum,
//     exclusiveMinimum, exclusiveMaximum, minLength, maxLength, pattern,
//     minItems, maxItems, multipleOf, uniqueItems) are removed;
//   - boolean schema nodes become an empty closed object.
//
// The input is never modified.
func Strictify(schema map[string]any) (map[string]any, error) {
	return StrictifyWithOptions(schema, StrictifyOptions{})
}

// StrictifyWithOptions applies Strictify with optional wire-preserving controls.
// The input remains unmodified, including its required arrays.
func StrictifyWithOptions(schema map[string]any, options StrictifyOptions) (map[string]any, error) {
	strict, violations, err := strictifyWithReport(schema, options)
	if err != nil {
		return nil, err
	}
	if len(violations) > 0 {
		return nil, &StrictifyError{Violations: violations}
	}
	return strict, nil
}

// StrictifyWithReport applies the same rewrite as [Strictify] but returns the
// rewritten schema together with the violations left in it, so callers can
// log or tolerate them. The error is reserved for input that cannot be
// processed at all: a nil or non-JSON schema, or a recursive "$ref".
func StrictifyWithReport(schema map[string]any) (map[string]any, []StrictViolation, error) {
	return strictifyWithReport(schema, StrictifyOptions{})
}

func strictifyWithReport(schema map[string]any, options StrictifyOptions) (map[string]any, []StrictViolation, error) {
	root, err := normalizeSchema(schema)
	if err != nil {
		return nil, nil, err
	}
	if !options.AssumeAllPropertiesRequired {
		makeOmittedNullable(root)
	}
	out, err := resolveRefs(root)
	if err != nil {
		return nil, nil, err
	}
	applyStrict(out, options.PreserveRequiredOrder)
	return out, StrictViolations(out), nil
}

// StrictViolations checks a schema offline against the documented OpenAI
// strict-mode rules without rewriting it: the root is an object, every object
// closes additionalProperties and requires every property, no unsupported
// keyword is present, every anyOf branch has a type or enum, and the nesting,
// property and enum-value limits hold. An empty result means it is accepted.
func StrictViolations(schema map[string]any) []StrictViolation {
	root, err := normalizeSchema(schema)
	if err != nil {
		return []StrictViolation{{Path: "$", Reason: err.Error()}}
	}
	var report strictReport
	if root["type"] != "object" {
		report.add("$", "the root must be an object (not a union)")
	}
	report.walk(root, "$", 0)
	if report.properties > strictMaxProperties {
		report.add("$", fmt.Sprintf("%d properties (at most %d)", report.properties, strictMaxProperties))
	}
	if report.enumValues > strictMaxEnumValues {
		report.add("$", fmt.Sprintf("%d enum values (at most %d)", report.enumValues, strictMaxEnumValues))
	}
	return report.violations
}

// normalizeSchema deep-copies schema through JSON so later rewrites never
// alias the caller's maps and every container is map[string]any or []any.
func normalizeSchema(schema map[string]any) (map[string]any, error) {
	if schema == nil {
		return nil, errors.New("tool: schema is nil")
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("tool: schema is not JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, fmt.Errorf("tool: schema is not JSON: %w", err)
	}
	return out, nil
}

// forEachSubschema replaces every direct child schema of node with the result
// of fn. Only keywords that hold schemas are visited, so enum, const and
// default values are never treated as schemas.
func forEachSubschema(node map[string]any, fn func(child any) any) {
	for _, key := range []string{"properties", "$defs", "definitions"} {
		if children, ok := node[key].(map[string]any); ok {
			for name, child := range children {
				children[name] = fn(child)
			}
		}
	}
	for _, key := range []string{"items", "not"} {
		switch child := node[key].(type) {
		case map[string]any, bool:
			node[key] = fn(child)
		}
	}
	for _, key := range []string{"items", "anyOf", "allOf", "oneOf", "prefixItems"} {
		if children, ok := node[key].([]any); ok {
			for i, child := range children {
				children[i] = fn(child)
			}
		}
	}
}

// makeOmittedNullable turns every property missing from "required" into
// anyOf[property, null]: under strict mode a field can only be omitted by
// sending null.
func makeOmittedNullable(value any) {
	node, ok := value.(map[string]any)
	if !ok {
		return
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		required := stringSet(node["required"])
		for name, property := range properties {
			if !required[name] && !alreadyNullable(property) {
				properties[name] = map[string]any{
					"anyOf": []any{property, map[string]any{"type": "null"}},
				}
			}
		}
	}
	forEachSubschema(node, func(child any) any {
		makeOmittedNullable(child)
		return child
	})
}

// anySlice returns value as a JSON array, or nil when it is not one.
func anySlice(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

// anyMap returns value as a JSON object, or nil when it is not one.
func anyMap(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return nil
}

func stringSet(value any) map[string]bool {
	set := map[string]bool{}
	for _, item := range anySlice(value) {
		if name, ok := item.(string); ok {
			set[name] = true
		}
	}
	return set
}

func alreadyNullable(schema any) bool {
	node, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	if types, ok := node["type"].([]any); ok {
		for _, kind := range types {
			if kind == "null" {
				return true
			}
		}
	}
	if node["type"] == "null" {
		return true
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		for _, branch := range anySlice(node[key]) {
			if branchNode, ok := branch.(map[string]any); ok && branchNode["type"] == "null" {
				return true
			}
		}
	}
	return false
}

// resolveRefs returns a copy of root with every local $ref inlined.
func resolveRefs(root map[string]any) (map[string]any, error) {
	resolved, err := resolveSchema(copyMap(root), root, nil)
	if err != nil {
		return nil, err
	}
	out, ok := resolved.(map[string]any)
	if !ok {
		return nil, errors.New("tool: schema root resolves to a non-object")
	}
	return out, nil
}

// resolveSchema inlines $ref in node (a private copy, rewritten in place).
// Definitions resolve against the untouched root; stack holds the refs being
// expanded so a cycle is reported instead of expanded forever.
func resolveSchema(node any, root map[string]any, stack []string) (any, error) {
	schema, ok := node.(map[string]any)
	if !ok {
		return node, nil
	}
	if ref, isRef := schema["$ref"].(string); isRef {
		return inlineRef(schema, ref, root, stack)
	}
	delete(schema, "$defs")
	delete(schema, "definitions")
	var firstErr error
	forEachSubschema(schema, func(child any) any {
		resolved, err := resolveSchema(child, root, stack)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return resolved
	})
	return schema, firstErr
}

// inlineRef replaces a $ref node by its resolved definition. Sibling keywords
// that travelled with the $ref are kept unless the definition sets them.
// Unresolvable references stay in place and surface as violations.
func inlineRef(node map[string]any, ref string, root map[string]any, stack []string) (any, error) {
	target, found := lookupRef(ref, root)
	if !found {
		return node, nil
	}
	if len(stack) >= strictRefMaxDepth || containsString(stack, ref) {
		return nil, fmt.Errorf("tool: recursive $ref %q cannot be inlined", ref)
	}
	resolved, err := resolveSchema(copyValue(target), root, append(stack, ref))
	if err != nil {
		return nil, err
	}
	merged, ok := resolved.(map[string]any)
	if !ok {
		return resolved, nil
	}
	for key, sibling := range node {
		if _, exists := merged[key]; key != "$ref" && !exists {
			merged[key] = sibling
		}
	}
	return merged, nil
}

// copyMap deep-copies a JSON container tree so two inlined uses of one
// definition never share maps that later rewrites mutate.
func copyMap(node map[string]any) map[string]any {
	out := make(map[string]any, len(node))
	for key, value := range node {
		out[key] = copyValue(value)
	}
	return out
}

func copyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyValue(item)
		}
		return out
	default:
		return value
	}
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// lookupRef resolves "#", "#/$defs/X" and "#/definitions/X" style local
// JSON pointers against the schema root.
func lookupRef(ref string, root any) (any, bool) {
	pointer, ok := strings.CutPrefix(ref, "#")
	if !ok {
		return nil, false
	}
	current := root
	for _, segment := range strings.Split(pointer, "/") {
		if segment == "" {
			continue
		}
		segment = strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		node, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = node[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}

// applyStrict enforces the strict object rules in place and recurses into
// every sub-schema. Boolean nodes are replaced, so callers pass the slot.
func applyStrict(node map[string]any, preserveRequiredOrder bool) {
	for _, key := range []string{"$schema", "title", "$defs", "definitions"} {
		delete(node, key)
	}
	convertArrayTypeToAnyOf(node)
	collapseOneOf(node)
	stripForbiddenKeywords(node)

	if properties, ok := node["properties"].(map[string]any); ok {
		required, keep := node["required"].([]any)
		keep = preserveRequiredOrder && keep && len(required) == len(properties)
		if keep {
			names := stringSet(required)
			keep = len(names) == len(properties)
			for name := range properties {
				if !names[name] {
					keep = false
					break
				}
			}
		}
		if !keep {
			names := make([]string, 0, len(properties))
			for name := range properties {
				names = append(names, name)
			}
			sort.Strings(names)
			required = make([]any, len(names))
			for i, name := range names {
				required[i] = name
			}
		}
		node["additionalProperties"] = false
		node["required"] = required
	}
	forEachSubschema(node, func(child any) any {
		switch typed := child.(type) {
		case bool:
			return map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
				"required":             []any{},
			}
		case map[string]any:
			applyStrict(typed, preserveRequiredOrder)
		}
		return child
	})
}

// typeKeywords are the keywords owned by one JSON type; they move into that
// type's anyOf branch when a type array is split.
var typeKeywords = map[string][]string{
	"array":  {"items", "prefixItems"},
	"object": {"properties", "required", "additionalProperties"},
}

func convertArrayTypeToAnyOf(node map[string]any) {
	types, ok := node["type"].([]any)
	if !ok {
		return
	}
	branches := make([]any, len(types))
	for i, kind := range types {
		name, isString := kind.(string)
		if !isString {
			branches[i] = kind
			continue
		}
		branch := map[string]any{}
		for _, keyword := range typeKeywords[name] {
			if value, present := node[keyword]; present {
				branch[keyword] = value
				delete(node, keyword)
			}
		}
		branch["type"] = name
		branches[i] = branch
	}
	delete(node, "type")
	node["anyOf"] = branches
}

// collapseOneOf merges a oneOf made only of enum/const branches into a single
// enum node and downgrades any other oneOf to anyOf.
func collapseOneOf(node map[string]any) {
	branches, ok := node["oneOf"].([]any)
	if !ok {
		return
	}
	var (
		values      []any
		enumType    any
		description any
		nonEnum     bool
	)
	for _, raw := range branches {
		branch, isMap := raw.(map[string]any)
		if !isMap {
			nonEnum = true
			continue
		}
		if enumType == nil {
			if kind, isString := branch["type"].(string); isString {
				enumType = kind
			}
		}
		if description == nil {
			description = branch["description"]
		}
		items, hasEnum := branch["enum"].([]any)
		values = append(values, items...)
		constant, hasConst := branch["const"]
		if hasConst {
			values = append(values, constant)
		}
		if !hasEnum && !hasConst {
			nonEnum = true
		}
	}
	delete(node, "oneOf")
	if len(values) == 0 || nonEnum {
		node["anyOf"] = branches
		return
	}
	if description == nil {
		description = node["description"]
	}
	for key := range node {
		delete(node, key)
	}
	if enumType != nil {
		node["type"] = enumType
	}
	node["enum"] = dedupeJSON(values)
	if description != nil {
		node["description"] = description
	}
}

func dedupeJSON(values []any) []any {
	seen := map[string]bool{}
	out := make([]any, 0, len(values))
	for _, value := range values {
		key, err := json.Marshal(value)
		if err != nil || seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		out = append(out, value)
	}
	return out
}

// strictStrippedKeywords are rewritten away because strict mode rejects them.
var strictStrippedKeywords = []string{
	"format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
	"minLength", "maxLength", "pattern", "minItems", "maxItems", "multipleOf",
	"uniqueItems", "default",
}

func stripForbiddenKeywords(node map[string]any) {
	if constant, ok := node["const"]; ok {
		delete(node, "const")
		if _, hasEnum := node["enum"]; !hasEnum {
			node["enum"] = []any{constant}
		}
	}
	for _, key := range strictStrippedKeywords {
		delete(node, key)
	}
}

// strictRejectedKeywords are reported by StrictViolations when present.
var strictRejectedKeywords = []string{
	"oneOf", "allOf", "not", "$ref", "$defs", "definitions", "const", "default",
	"format", "minimum", "maximum", "minLength", "maxLength", "pattern",
	"minItems", "maxItems", "patternProperties", "if", "then", "else",
}

type strictReport struct {
	violations []StrictViolation
	properties int
	enumValues int
}

func (r *strictReport) add(path, reason string) {
	r.violations = append(r.violations, StrictViolation{Path: path, Reason: reason})
}

func (r *strictReport) walk(raw any, path string, depth int) {
	node, ok := raw.(map[string]any)
	if !ok {
		r.add(path, "a schema node must be an object")
		return
	}
	for _, key := range strictRejectedKeywords {
		if _, present := node[key]; present {
			r.add(path, fmt.Sprintf("`%s` is not supported", key))
		}
	}
	if _, isArray := node["type"].([]any); isArray {
		r.add(path, "an array `type` (use anyOf)")
	}
	if values, ok := node["enum"].([]any); ok {
		if len(values) == 0 {
			r.add(path, "an empty enum")
		}
		r.enumValues += len(values)
	}
	if node["type"] == "object" {
		r.walkObject(node, path, depth+1)
		return
	}
	if items, ok := node["items"]; ok {
		r.walk(items, path+"[]", depth)
	}
	if branches, ok := node["anyOf"].([]any); ok {
		for i, branch := range branches {
			at := fmt.Sprintf("%s.anyOf[%d]", path, i)
			if bm, isMap := branch.(map[string]any); isMap && bm["type"] == nil && bm["enum"] == nil {
				r.add(at, "a branch without a type or an enum")
			}
			r.walk(branch, at, depth)
		}
	} else if node["type"] == nil && node["enum"] == nil {
		r.add(path, "a node without a type")
	}
}

func (r *strictReport) walkObject(node map[string]any, path string, depth int) {
	if depth > strictMaxDepth {
		r.add(path, fmt.Sprintf("nested %d objects deep (at most %d)", depth, strictMaxDepth))
	}
	if node["additionalProperties"] != false {
		r.add(path, "additionalProperties must be false")
	}
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		r.add(path, "an object without properties")
		return
	}
	r.properties += len(properties)
	required := stringSet(node["required"])
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !required[name] {
			r.add(path, fmt.Sprintf("`%s` is not required", name))
		}
	}
	for _, name := range names {
		r.walk(properties[name], path+"."+name, depth)
	}
}

// CoerceEmptyObjectsToNull repairs model output decoded as JSON (map[string]any
// and []any): wherever the schema position is nullable (an anyOf branch of
// type null) and the value is an empty object, the value becomes nil. Some
// models answer an unfillable nullable field with {} instead of null. The
// walk follows properties and items; schema must come from [Strictify]. Nested
// containers are modified in place; the returned value replaces value (it
// differs only when the root itself is rewritten) and the count is the number
// of rewrites.
func CoerceEmptyObjectsToNull(value any, schema map[string]any) (any, int) {
	return coerceEmptyObjects(value, schema, func(schema map[string]any) (any, bool) {
		for _, branch := range anySlice(schema["anyOf"]) {
			if node, ok := branch.(map[string]any); ok && node["type"] == "null" {
				return nil, true
			}
		}
		return nil, false
	})
}

// CoerceEmptyObjectsToEmptyString is the companion of
// [CoerceEmptyObjectsToNull] for non-nullable string positions: an empty
// object where the schema says {"type":"string"} becomes "". Nullable
// positions (anyOf) are left for the null coercion.
func CoerceEmptyObjectsToEmptyString(value any, schema map[string]any) (any, int) {
	return coerceEmptyObjects(value, schema, func(schema map[string]any) (any, bool) {
		if schema["type"] == "string" {
			return "", true
		}
		return nil, false
	})
}

func coerceEmptyObjects(value any, schema map[string]any, replace func(map[string]any) (any, bool)) (any, int) {
	if schema == nil {
		return value, 0
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			if replacement, ok := replace(schema); ok {
				return replacement, 1
			}
			return value, 0
		}
		return value, coerceObjectFields(typed, schema, replace)
	case []any:
		items := anyMap(schema["items"])
		total := 0
		for i, item := range typed {
			var n int
			typed[i], n = coerceEmptyObjects(item, items, replace)
			total += n
		}
		return value, total
	default:
		return value, 0
	}
}

func coerceObjectFields(value, schema map[string]any, replace func(map[string]any) (any, bool)) int {
	properties := anyMap(schema["properties"])
	total := 0
	for name, field := range value {
		fieldSchema := anyMap(properties[name])
		var n int
		value[name], n = coerceEmptyObjects(field, fieldSchema, replace)
		total += n
	}
	return total
}
