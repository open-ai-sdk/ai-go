package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

var errApprovalPending = errors.New("agent: tool approval pending")

const minApprovalKeyBytes = 32

var (
	// ErrInvalidApprovalSignature reports a forged, altered, or unsigned
	// history-based approval response.
	ErrInvalidApprovalSignature = errors.New("agent: invalid tool approval signature")
	// ErrApprovalReplay reports a previously consumed approval capability.
	ErrApprovalReplay = errors.New("agent: tool approval capability already consumed")
)

func newApprovalID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("agent: generate approval ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// ApprovalRequest describes a suspended tool call awaiting a decision.
// ApprovalID correlates the request with its response.
type ApprovalRequest struct{ ApprovalID, ToolCallID, ToolName, Args string }

// ApprovalResponse carries the decision for one ApprovalRequest.
// A call proceeds only when Approved is true; the default is deny.
type ApprovalResponse struct {
	ApprovalID string
	Approved   bool
	Reason     string
}

// ApprovalResponder optionally resolves an approval request within the current
// invocation. When it is nil, the run suspends after emitting the request; the
// caller resumes by adding a signed tool_approval_response content part to the
// next request's message history.
type ApprovalResponder interface {
	RequestApproval(context.Context, ApprovalRequest) (ApprovalResponse, error)
}

// ApprovalGrant is the authenticated capability consumed before an approved
// history response can execute a tool. CapabilityID is the v1 HMAC signature.
type ApprovalGrant struct {
	CapabilityID  string
	ToolCallID    string
	canonicalArgs string
}

// ApprovalReservation owns an atomic batch claim. Complete permanently consumes
// one capability after its tool attempt; Release relinquishes every incomplete
// claim so cancellation before execution remains retryable.
type ApprovalReservation interface {
	Complete(context.Context, ApprovalGrant) error
	Release(context.Context) error
}

// ApprovalReplayGuard atomically reserves a batch of approval capabilities.
// Durable implementations should use expiring leases/fencing so process death
// releases unfinished claims while completed grants remain at-most-once.
type ApprovalReplayGuard interface {
	ReserveApprovals(context.Context, []ApprovalGrant) (ApprovalReservation, error)
}

// MemoryApprovalReplayGuard is a process-local replay guard for tests,
// development, and bounded-lifetime jobs. Completed capability IDs are retained
// until the guard is discarded, so production servers should implement
// ApprovalReplayGuard with a bounded shared transactional lease store.
type MemoryApprovalReplayGuard struct {
	mu       sync.Mutex
	claimed  map[string]struct{}
	consumed map[string]struct{}
}

func NewMemoryApprovalReplayGuard() *MemoryApprovalReplayGuard {
	return &MemoryApprovalReplayGuard{
		claimed: make(map[string]struct{}), consumed: make(map[string]struct{}),
	}
}

func (g *MemoryApprovalReplayGuard) ReserveApprovals(
	ctx context.Context,
	grants []ApprovalGrant,
) (ApprovalReservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return &memoryApprovalReservation{guard: g}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	batch := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		if _, duplicate := batch[grant.CapabilityID]; duplicate {
			return nil, fmt.Errorf("%w: capability repeated in batch", ErrApprovalReplay)
		}
		if _, exists := g.consumed[grant.CapabilityID]; exists {
			return nil, fmt.Errorf("%w: tool call %q", ErrApprovalReplay, grant.ToolCallID)
		}
		if _, exists := g.claimed[grant.CapabilityID]; exists {
			return nil, fmt.Errorf("%w: tool call %q is already reserved", ErrApprovalReplay, grant.ToolCallID)
		}
		batch[grant.CapabilityID] = struct{}{}
	}
	for _, grant := range grants {
		g.claimed[grant.CapabilityID] = struct{}{}
	}
	return &memoryApprovalReservation{
		guard: g, pending: batch,
	}, nil
}

type memoryApprovalReservation struct {
	mu       sync.Mutex
	guard    *MemoryApprovalReplayGuard
	pending  map[string]struct{}
	released bool
}

func (r *memoryApprovalReservation) Complete(ctx context.Context, grant ApprovalGrant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released {
		return errors.New("agent: approval reservation already released")
	}
	if _, ok := r.pending[grant.CapabilityID]; !ok {
		return fmt.Errorf("agent: approval capability %q is not pending in this reservation", grant.ToolCallID)
	}
	r.guard.mu.Lock()
	delete(r.guard.claimed, grant.CapabilityID)
	r.guard.consumed[grant.CapabilityID] = struct{}{}
	r.guard.mu.Unlock()
	delete(r.pending, grant.CapabilityID)
	return nil
}

func (r *memoryApprovalReservation) Release(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released {
		return nil
	}
	r.guard.mu.Lock()
	for capability := range r.pending {
		delete(r.guard.claimed, capability)
	}
	r.guard.mu.Unlock()
	r.pending = nil
	r.released = true
	return nil
}

// signApprovalRequest implements AI SDK v7's exact payload:
// ["ai-sdk-tool-approval-v1", approvalId, toolCallId, toolName,
// hashCanonical(input)]. Approved is intentionally absent: the signature
// proves the server gated this tool/input, not that a user approved it.
func signApprovalRequest(key []byte, request ApprovalRequest) (string, error) {
	if len(key) < minApprovalKeyBytes {
		return "", fmt.Errorf("agent: ToolApprovalKey must contain at least %d bytes", minApprovalKeyBytes)
	}
	_, inputDigest, err := canonicalizeApprovalInput(request.Args)
	if err != nil {
		return "", fmt.Errorf("agent: canonicalize approval input: %w", err)
	}
	payload := "[" + strings.Join([]string{
		jsonString("ai-sdk-tool-approval-v1"),
		jsonString(request.ApprovalID),
		jsonString(request.ToolCallID),
		jsonString(request.ToolName),
		jsonString(inputDigest),
	}, ",") + "]"
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyApprovalRequest(
	key []byte,
	request ApprovalRequest,
	signature string,
) (ApprovalGrant, error) {
	canonicalArgs, _, err := canonicalizeApprovalInput(request.Args)
	if err != nil {
		return ApprovalGrant{}, fmt.Errorf("agent: canonicalize approval input: %w", err)
	}
	expected, err := signApprovalRequest(key, request)
	if err != nil {
		return ApprovalGrant{}, err
	}
	expectedBytes, _ := base64.RawURLEncoding.DecodeString(expected)
	actualBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(actualBytes, expectedBytes) {
		return ApprovalGrant{}, ErrInvalidApprovalSignature
	}
	return ApprovalGrant{
		CapabilityID: signature, ToolCallID: request.ToolCallID, canonicalArgs: canonicalArgs,
	}, nil
}

// signApprovalResult creates an internal continuation receipt. It is distinct
// from the AI SDK v1 request signature above: the request signature is echoed
// by an untrusted client, while this receipt is emitted only after the runtime
// has resolved that signed request. This lets later clean continuations omit
// the consumed approval-response message without trusting forged tool results.
func signApprovalResult(
	key []byte,
	request ApprovalRequest,
	approved bool,
	modelOutput string,
) (string, error) {
	if len(key) < minApprovalKeyBytes {
		return "", fmt.Errorf("agent: ToolApprovalKey must contain at least %d bytes", minApprovalKeyBytes)
	}
	requestSignature, err := signApprovalRequest(key, request)
	if err != nil {
		return "", err
	}
	outputDigest := sha256.Sum256([]byte(modelOutput))
	payload := "[" + strings.Join([]string{
		jsonString("ai-go-tool-approval-result-v1"),
		jsonString(requestSignature),
		jsonString(request.ApprovalID),
		jsonString(request.ToolCallID),
		jsonString(request.ToolName),
		fmt.Sprintf("%t", approved),
		jsonString(base64.RawURLEncoding.EncodeToString(outputDigest[:])),
	}, ",") + "]"
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyApprovalResult(
	key []byte,
	request ApprovalRequest,
	approved bool,
	modelOutput string,
	receipt string,
) error {
	expected, err := signApprovalResult(key, request, approved, modelOutput)
	if err != nil {
		return err
	}
	expectedBytes, _ := base64.RawURLEncoding.DecodeString(expected)
	actualBytes, err := base64.RawURLEncoding.DecodeString(receipt)
	if err != nil || !hmac.Equal(actualBytes, expectedBytes) {
		return ErrInvalidApprovalSignature
	}
	return nil
}

func hashCanonicalJSON(raw string) (string, error) {
	_, digest, err := canonicalizeApprovalInput(raw)
	return digest, err
}

func canonicalizeApprovalInput(raw string) (string, string, error) {
	if err := validateApprovalJSONUnicode(raw); err != nil {
		return "", "", err
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", "", err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return "", "", errors.New("multiple JSON values")
		}
		return "", "", err
	}
	var canonical bytes.Buffer
	if err := writeCanonicalJSON(&canonical, value); err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(canonical.Bytes())
	return canonical.String(), base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

// validateApprovalJSONUnicode rejects inputs that encoding/json would silently
// repair to U+FFFD. Unpaired UTF-16 escapes are not interoperable JSON (I-JSON)
// and would otherwise create a Go/JavaScript signature differential.
func validateApprovalJSONUnicode(raw string) error {
	if !utf8.ValidString(raw) {
		return errors.New("approval input is not valid UTF-8")
	}
	inString := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || i+1 >= len(raw) {
				continue
			}
			if raw[i+1] != 'u' {
				i++
				continue
			}
			if i+6 > len(raw) {
				return errors.New("incomplete unicode escape in approval input")
			}
			unit, err := strconv.ParseUint(raw[i+2:i+6], 16, 16)
			if err != nil {
				return fmt.Errorf("invalid unicode escape in approval input: %w", err)
			}
			if 0xDC00 <= unit && unit <= 0xDFFF {
				return errors.New("unpaired low surrogate in approval input")
			}
			if 0xD800 <= unit && unit <= 0xDBFF {
				if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
					return errors.New("unpaired high surrogate in approval input")
				}
				low, err := strconv.ParseUint(raw[i+8:i+12], 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return errors.New("unpaired high surrogate in approval input")
				}
				i += 11
				continue
			}
			i += 5
		}
	}
	return nil
}

func writeCanonicalJSON(out *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if value {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case float64:
		number, err := jcs.NumberToJSON(value)
		if err != nil {
			return err
		}
		out.WriteString(number)
	case json.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		if err != nil {
			var numErr *strconv.NumError
			if !errors.As(err, &numErr) || numErr.Err != strconv.ErrRange {
				return err
			}
		}
		// JSON.parse permits exponents outside the finite IEEE-754 range and
		// produces +/-Infinity; JSON.stringify, which the v1 contract uses,
		// serializes those non-finite values as null.
		if math.IsInf(number, 0) || math.IsNaN(number) {
			out.WriteString("null")
			break
		}
		encoded, err := jcs.NumberToJSON(number)
		if err != nil {
			return err
		}
		out.WriteString(encoded)
	case string:
		out.WriteString(jsonString(value))
	case []any:
		out.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonicalJSON(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return utf16Less(keys[i], keys[j])
		})
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			out.WriteString(jsonString(key))
			out.WriteByte(':')
			if err := writeCanonicalJSON(out, value[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("unsupported JSON value %T", value)
	}
	return nil
}

func utf16Less(left, right string) bool {
	l := utf16.Encode([]rune(left))
	r := utf16.Encode([]rune(right))
	for i := 0; i < len(l) && i < len(r); i++ {
		if l[i] != r[i] {
			return l[i] < r[i]
		}
	}
	return len(l) < len(r)
}

func jsonString(value string) string {
	var out strings.Builder
	out.Grow(len(value) + 2)
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&out, `\u%04x`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}
