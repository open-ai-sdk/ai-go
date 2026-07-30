package aisdk

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// CanonicalJSONMaxDepth bounds recursion in CanonicalJSON.
//
// This is a security bound, not a sanity check. CanonicalJSON is the FIRST code to touch
// attacker-supplied JSON on the approval path — validate-tool-approvals.ts fixes the
// order as signature check, then schema, then policy, and the signature check hashes the
// input. The reference's canonicalJSON is unguarded recursion, so a deeply nested body
// would exhaust the stack before anything authenticated it.
const CanonicalJSONMaxDepth = 64

// CanonicalJSON produces the deterministic serialization the AI SDK v7 approval
// signature hashes, byte-for-byte identical to
// ai/src/util/canonical-hash.ts#canonicalJSON.
//
// Three things make this not a one-liner over encoding/json:
//
//   - Key order must be UTF-16 code-unit order, because that is what JavaScript's
//     Array.prototype.sort does. Go's sort.Strings is UTF-8 byte order, and the two
//     disagree: measured, sort.Strings gives ["a", U+E000, U+1F600] where JS gives
//     ["a", U+1F600, U+E000]. A non-BMP key next to a U+E000..U+FFFF key is the case
//     that diverges; an emoji next to a Latin-1 key agrees in both, so testing only
//     that would ship the bug.
//   - HTML must not be escaped. Go's encoder turns < > & into < > & by
//     default; JSON.stringify leaves them alone.
//   - Numbers must be formatted by encoding/json, not strconv. Measured identical to
//     JSON.stringify across 1e21, 1e-7, 5e-324 and friends; strconv.FormatFloat is not.
func CanonicalJSON(value any) (string, error) {
	var sb strings.Builder
	if err := writeCanonical(&sb, value, 0); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func writeCanonical(sb *strings.Builder, value any, depth int) error {
	if depth > CanonicalJSONMaxDepth {
		return errCanonicalDepth(depth)
	}

	switch v := value.(type) {
	case nil:
		sb.WriteString("null")
		return nil

	case map[string]any:
		return writeCanonicalObject(sb, v, depth)

	case []any:
		sb.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				sb.WriteByte(',')
			}
			if err := writeCanonical(sb, item, depth+1); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
		return nil

	case string:
		enc, err := encodeScalar(v)
		if err != nil {
			return err
		}
		sb.WriteString(enc)
		return nil

	default:
		// Numbers, bools, and anything else already in the JSON data model. Values
		// outside it are normalized by HashCanonical before reaching here.
		enc, err := encodeScalar(v)
		if err != nil {
			return err
		}
		sb.WriteString(enc)
		return nil
	}
}

func writeCanonicalObject(sb *strings.Builder, obj map[string]any, depth int) error {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sortUTF16(keys)

	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		enc, err := encodeScalar(k)
		if err != nil {
			return err
		}
		sb.WriteString(enc)
		sb.WriteByte(':')
		if err := writeCanonical(sb, obj[k], depth+1); err != nil {
			return err
		}
	}
	sb.WriteByte('}')
	return nil
}

// encodeScalar renders one JSON scalar exactly as JSON.stringify would.
//
// Invalid UTF-8 is rejected rather than encoded. Go's encoder would silently substitute
// U+FFFD, and that substitution is lossy in a way that matters here: "\uD800" and
// "\uD801" both become U+FFFD, so two distinct inputs would hash identically. For a
// value feeding a signature that is a collision, not a formatting quirk. JSON.stringify
// emits lone surrogates verbatim, so Go cannot reproduce the reference bytes either —
// refusing is the only honest option.
func encodeScalar(v any) (string, error) {
	if s, ok := v.(string); ok && !utf8.ValidString(s) {
		return "", errCanonicalInvalidUTF8(s)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", errCanonicalEncode(err)
	}
	// Encode appends a newline; canonical output has none.
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// sortUTF16 orders strings by UTF-16 code unit, matching JavaScript's default string
// comparison. See CanonicalJSON's doc comment for why byte order will not do.
func sortUTF16(keys []string) {
	sort.Slice(keys, func(i, j int) bool { return lessUTF16(keys[i], keys[j]) })
}

func lessUTF16(a, b string) bool {
	// Fast path: while both strings are ASCII, byte order and UTF-16 order agree, and
	// ASCII keys are the overwhelmingly common case.
	if isASCII(a) && isASCII(b) {
		return a < b
	}
	au := utf16.Encode([]rune(a))
	bu := utf16.Encode([]rune(b))
	for i := 0; i < len(au) && i < len(bu); i++ {
		if au[i] != bu[i] {
			return au[i] < bu[i]
		}
	}
	return len(au) < len(bu)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// normalizeToJSONModel converts an arbitrary Go value into the JSON data model — the
// shape a decoded request body already has.
//
// Numbers become float64 so there is one numeric type, as in JavaScript. Without this an
// int64 and a float64 holding the same value would canonicalize differently, and the
// same logical input would produce two different signatures depending on how the caller
// happened to build it.
func normalizeToJSONModel(value any) (any, error) {
	switch value.(type) {
	case nil, bool, string, float64, map[string]any, []any:
		// Already in the model; avoid a marshal round-trip that would only risk
		// changing it.
		return value, nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, errCanonicalEncode(err)
	}

	var out any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	if err := dec.Decode(&out); err != nil {
		return nil, errCanonicalEncode(err)
	}
	return out, nil
}
