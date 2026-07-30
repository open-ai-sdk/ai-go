package aisdk

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// HashCanonical is the base64url SHA-256 of a value's canonical JSON, matching
// ai/src/util/canonical-hash.ts#hashCanonical.
//
// The value is normalized into the JSON data model first so an int and a float holding
// the same number produce the same digest — JavaScript has one numeric type, and a
// signature that depended on how the caller happened to build its input would be
// unusable.
func HashCanonical(value any) (string, error) {
	normalized, err := normalizeToJSONModel(value)
	if err != nil {
		return "", err
	}
	canonical, err := CanonicalJSON(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return EncodeBase64URL(sum[:]), nil
}

// EncodeBase64URL matches the reference's toBase64url: base64 with -/_ and no padding.
//
// RawURLEncoding already omits padding; do not reach for URLEncoding and strip '='
// manually, which is where an off-by-one on the padding length usually creeps in.
func EncodeBase64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeBase64URLTolerant decodes a signature the way the reference client does.
//
// This has to be lenient, and the reason is not aesthetic. The reference decodes with
// convertBase64ToUint8Array (provider-utils/src/uint8-utils.ts:6-10): it rewrites '-'→'+'
// and '_'→'/' and then calls atob, which accepts '=' padding, standard-base64 alphabet,
// and an unpadded final group. Go's base64.RawURLEncoding rejects padding and '+'/'/';
// base64.StdEncoding rejects '-'/'_'. Measured: "AAA=" and "a+/b" fail RawURLEncoding,
// "a-_b" and "AAA" fail StdEncoding.
//
// So a signature that the reference verifies would fail here for a purely lexical reason.
// Being STRICTER than the reference is safe from a security standpoint but breaks
// interoperability; being LOOSER would be the dangerous direction, and normalizing the
// alphabet does not loosen anything — it cannot make a wrong MAC verify.
//
// ok is false for anything undecodable. There is deliberately no error return: the
// caller is a boolean verify, and an error there invites a caller to treat a decode
// failure as something other than "did not verify".
func DecodeBase64URLTolerant(s string) (b []byte, ok bool) {
	if s == "" {
		return nil, false
	}

	// Normalize to the standard alphabet, matching the reference's replace pair.
	normalized := strings.NewReplacer("-", "+", "_", "/").Replace(s)
	// Strip padding and decode raw, so a padded and an unpadded form of the same
	// signature both land on the same bytes.
	normalized = strings.TrimRight(normalized, "=")

	decoded, err := base64.RawStdEncoding.DecodeString(normalized)
	if err != nil {
		return nil, false
	}
	// A padding-only input trims to "", which RawStdEncoding decodes happily to zero
	// bytes. The reference throws on it (atob("====") raises), and a zero-length
	// signature cannot be a 32-byte HMAC, so report it as undecodable rather than as an
	// empty success.
	if len(decoded) == 0 {
		return nil, false
	}
	return decoded, true
}
