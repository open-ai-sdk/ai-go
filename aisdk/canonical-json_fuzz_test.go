package aisdk

import (
	"encoding/json"
	"testing"
)

// FuzzCanonicalJSON asserts the properties that make the canonical form usable as a
// signature input, on arbitrary JSON rather than hand-picked vectors.
//
// Three invariants, each of which would be a real defect if broken:
//
//  1. It never panics. This runs on pre-authentication input, so a panic is a denial of
//     service reachable by an unauthenticated request.
//  2. It is deterministic. Go map iteration order is randomized per run, so a missing
//     sort would show up here as the same input producing two different signatures.
//  3. Its output is valid JSON that round-trips to an equal value — the canonical form
//     is a re-encoding, not a lossy digest of its own.
func FuzzCanonicalJSON(f *testing.F) {
	seeds := []string{
		`{}`, `[]`, `null`, `0`, `""`, `true`,
		`{"a":1,"b":[1,2,{"c":3}]}`,
		`{"😀":1,"":2,"a":3}`,
		`{"<":"&",">":"\""}`,
		`[1e21,1e-7,5e-324,-0.0]`,
		`{"Tiếng":"Việt","日本":"語"}`,
		`{"":""}`,
		`[[[[[[[[[[1]]]]]]]]]]`,
		`{"a":{"a":{"a":{"a":null}}}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			// Only well-formed JSON is in scope; the decoder is not under test.
			t.Skip()
		}

		first, err := CanonicalJSON(value)
		if err != nil {
			// Refusing is a valid outcome — depth budget, invalid UTF-8. It must be the
			// typed error, though, not something that leaked from encoding/json.
			var cErr *CanonicalJSONError
			if !asCanonicalError(err, &cErr) {
				t.Fatalf("CanonicalJSON returned an untyped error: %v", err)
			}
			return
		}

		// Determinism across runs with a re-decoded value, so map iteration order differs.
		var again any
		if err := json.Unmarshal([]byte(raw), &again); err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		second, err := CanonicalJSON(again)
		if err != nil {
			t.Fatalf("second canonicalization failed after the first succeeded: %v", err)
		}
		if first != second {
			t.Fatalf("not deterministic:\n 1: %s\n 2: %s", first, second)
		}

		// The output must itself be valid JSON denoting the same value.
		var roundTripped any
		if err := json.Unmarshal([]byte(first), &roundTripped); err != nil {
			t.Fatalf("canonical output is not valid JSON: %s (%v)", first, err)
		}
		reCanonical, err := CanonicalJSON(roundTripped)
		if err != nil {
			t.Fatalf("canonicalizing the canonical form failed: %v", err)
		}
		if reCanonical != first {
			t.Fatalf("not idempotent:\n in: %s\nout: %s", first, reCanonical)
		}

		// And the digest must follow the same rules.
		if _, err := HashCanonical(value); err != nil {
			t.Fatalf("HashCanonical failed on a value CanonicalJSON accepted: %v", err)
		}
	})
}

// asCanonicalError is errors.As without importing errors into the fuzz file's surface.
func asCanonicalError(err error, target **CanonicalJSONError) bool {
	for e := err; e != nil; {
		if c, ok := e.(*CanonicalJSONError); ok {
			*target = c
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// FuzzVerifyToolApproval asserts the verifier never panics and never accepts a signature
// under a secret that did not produce it, on arbitrary bytes.
func FuzzVerifyToolApproval(f *testing.F) {
	f.Add("secret", "sig", "a1", "c1", "tool", `{"a":1}`)
	f.Add("", "", "", "", "", `null`)
	f.Add("s", "====", "a", "c", "t", `{}`)
	f.Add("s", "!!!!", "a", "c", "t", `[]`)

	f.Fuzz(func(t *testing.T, secret, sig, approvalID, callID, toolName, rawInput string) {
		var input any
		if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
			t.Skip()
		}
		b := ApprovalBinding{
			ApprovalID: approvalID, ToolCallID: callID, ToolName: toolName, Input: input,
		}

		// Must not panic on any input.
		got := VerifyToolApproval([]byte(secret), sig, b)

		// A fuzzer will not stumble onto a valid HMAC, so any accept is a bug — unless
		// the fuzzed signature happens to equal the real one, which we check for rather
		// than assume away.
		if got {
			want, err := SignToolApprovalV2([]byte(secret), b)
			if err != nil {
				t.Fatalf("verify accepted a signature that signing cannot produce: %v", err)
			}
			wantV1, err1 := SignToolApprovalV1([]byte(secret), b)
			if err1 != nil {
				t.Fatalf("v1 signing failed: %v", err1)
			}
			pv, okv := DecodeBase64URLTolerant(sig)
			p2, _ := DecodeBase64URLTolerant(want)
			p1, _ := DecodeBase64URLTolerant(wantV1)
			if !okv || (!bytesEqual(pv, p2) && !bytesEqual(pv, p1)) {
				t.Fatalf("verify accepted a signature matching neither v2 nor v1")
			}
		}
	})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
