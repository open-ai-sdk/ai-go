package agent

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/open-ai-sdk/ai-go/aikit"
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
	return aikit.SignToolApproval(key, approvalSignatureInput(request))
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
	if err := aikit.VerifyToolApproval(key, signature, approvalSignatureInput(request)); err != nil {
		return ApprovalGrant{}, ErrInvalidApprovalSignature
	}
	return ApprovalGrant{
		CapabilityID: signature, ToolCallID: request.ToolCallID, canonicalArgs: canonicalArgs,
	}, nil
}

func approvalSignatureInput(request ApprovalRequest) aikit.ToolApprovalSignatureInput {
	return aikit.ToolApprovalSignatureInput{
		ApprovalID: request.ApprovalID, ToolCallID: request.ToolCallID,
		ToolName: request.ToolName, Input: json.RawMessage(request.Args),
	}
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
	expectedBytes, err := base64.RawURLEncoding.DecodeString(expected)
	if err != nil {
		return fmt.Errorf("agent: decode generated approval receipt: %w", err)
	}
	actualBytes, err := base64.RawURLEncoding.DecodeString(receipt)
	if err != nil || !hmac.Equal(actualBytes, expectedBytes) {
		return ErrInvalidApprovalSignature
	}
	return nil
}

func canonicalizeApprovalInput(raw string) (string, string, error) {
	canonical, err := aikit.CanonicalizeToolApprovalInput(json.RawMessage(raw))
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(canonical)
	return string(canonical), base64.RawURLEncoding.EncodeToString(digest[:]), nil
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
