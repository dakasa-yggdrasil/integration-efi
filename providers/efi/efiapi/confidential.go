package efiapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// MaxConfidentialResponseBytes bounds a complete provider response before JSON
// decoding. The confidential output envelope has its own complete-message bound.
// Neither boundary may truncate a response and then treat it as valid evidence.
const MaxConfidentialResponseBytes = 512 * 1024

var (
	ErrConfidentialRequest           = errors.New("efi: confidential provider request failed")
	ErrConfidentialResponseTooLarge  = errors.New("efi: confidential provider response exceeds the limit")
	ErrConfidentialMutationRefused   = errors.New("efi: confidential provider mutation refused")
	ErrConfidentialMutationSucceeded = errors.New("efi: confidential provider accepted mutation but its response is unproven")
)

type confidentialDiagnosticsKey struct{}
type mutationGuardKey struct{}
type mutationTimeoutKey struct{}

// WithConfidentialDiagnostics prevents arbitrary provider, network and JSON
// diagnostics from leaving this lower boundary, including through tracing. It
// deliberately carries no private provider value in its public API.
func WithConfidentialDiagnostics(ctx context.Context) context.Context {
	return context.WithValue(ctx, confidentialDiagnosticsKey{}, true)
}

func IsConfidential(ctx context.Context) bool {
	value, _ := ctx.Value(confidentialDiagnosticsKey{}).(bool)
	return value
}

// MutationSucceeded reports that the confidential PUT received HTTP2xx headers
// before response reading/decoding failed. It does not establish canonical GET
// proof. The reconciler must retain its safe event attempt and held outcome;
// neither this marker nor its cause authorizes replay or success evidence.
func MutationSucceeded(err error) bool {
	return errors.Is(err, ErrConfidentialMutationSucceeded)
}

// WithMutationGuard installs the invocation's source-owned authorization check.
// It is called immediately before every confidential HTTP PUT send, including a
// capability's separate v3 fallback. A missing guard refuses confidential PUTs.
// The callback must not do provider I/O; returned diagnostics are discarded.
func WithMutationGuard(ctx context.Context, guard func() error) context.Context {
	return context.WithValue(ctx, mutationGuardKey{}, guard)
}

// WithMutationTimeout bounds only an authorized PUT request, preserving the
// original context for later read-only confirmation and safe event/seal work.
// The callback uses the same invocation clock as the authorization guard.
func WithMutationTimeout(ctx context.Context, remaining func() time.Duration) context.Context {
	return context.WithValue(ctx, mutationTimeoutKey{}, remaining)
}

func mutationRequestContext(ctx context.Context, method string) (context.Context, context.CancelFunc, error) {
	if !IsConfidential(ctx) || method != http.MethodPut {
		return ctx, func() {}, nil
	}
	remaining, ok := ctx.Value(mutationTimeoutKey{}).(func() time.Duration)
	if !ok {
		return ctx, func() {}, nil
	} // Existing guard-only API; production sets both.
	if remaining == nil {
		return ctx, func() {}, ErrConfidentialMutationRefused
	}
	duration := remaining()
	if duration <= 0 || duration > 5*time.Minute+30*time.Second {
		return ctx, func() {}, ErrConfidentialMutationRefused
	}
	send, cancel := context.WithTimeout(ctx, duration)
	return send, cancel, nil
}

func confidentialMutationAllowed(ctx context.Context, method string) error {
	if !IsConfidential(ctx) || method != http.MethodPut {
		return nil
	}
	guard, ok := ctx.Value(mutationGuardKey{}).(func() error)
	if !ok || guard == nil || ctx.Err() != nil {
		return ErrConfidentialMutationRefused
	}
	if err := guard(); err != nil {
		return ErrConfidentialMutationRefused
	}
	return nil
}

// safeConfidentialError must run before RecordError or any outer RPC/logger
// boundary. Preserve only HTTP status needed for the existing v2/v3 fallback;
// retaining an untrusted cause via Unwrap would defeat this boundary.
func safeConfidentialError(ctx context.Context, err error) error {
	if err == nil || !IsConfidential(ctx) {
		return err
	}
	var apiErr *EfiAPIError
	if errors.As(err, &apiErr) {
		return newEfiAPIError(apiErr.Status, "provider_error")
	}
	switch {
	case MutationSucceeded(err):
		return ErrConfidentialMutationSucceeded
	case errors.Is(err, ErrConfidentialResponseTooLarge):
		return ErrConfidentialResponseTooLarge
	case errors.Is(err, ErrConfidentialMutationRefused):
		return ErrConfidentialMutationRefused
	default:
		return ErrConfidentialRequest
	}
}

func confidentialResponseFailure(ctx context.Context, method string, status int, err error) error {
	if err != nil && IsConfidential(ctx) && method == http.MethodPut && status >= 200 && status < 300 {
		return ErrConfidentialMutationSucceeded
	}
	return err
}

func readProviderResponse(ctx context.Context, body io.Reader) ([]byte, error) {
	if !IsConfidential(ctx) {
		// Preserve the existing legacy boundary and behavior.
		return io.ReadAll(io.LimitReader(body, 1024*1024))
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxConfidentialResponseBytes+1))
	if err != nil {
		return nil, ErrConfidentialRequest
	}
	if len(raw) > MaxConfidentialResponseBytes {
		return nil, ErrConfidentialResponseTooLarge
	}
	return raw, nil
}
