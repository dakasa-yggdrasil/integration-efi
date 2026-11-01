package efiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type confidentialTraceRecorder struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (r *confidentialTraceRecorder) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, spans...)
	return nil
}

func (*confidentialTraceRecorder) Shutdown(context.Context) error { return nil }

func (r *confidentialTraceRecorder) assertNoPrivateDiagnostics(t *testing.T, private []string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.spans) == 0 {
		t.Fatal("actual client path recorded no span")
	}
	var observed strings.Builder
	for _, span := range r.spans {
		observed.WriteString(span.Name())
		observed.WriteString(span.Status().Description)
		for _, attr := range span.Attributes() {
			observed.WriteString(string(attr.Key))
			observed.WriteString(attr.Value.Emit())
		}
		for _, event := range span.Events() {
			observed.WriteString(event.Name)
			for _, attr := range event.Attributes {
				observed.WriteString(string(attr.Key))
				observed.WriteString(attr.Value.Emit())
			}
		}
	}
	for _, value := range private {
		if strings.Contains(observed.String(), value) {
			t.Fatal("private diagnostic reached an actual trace span")
		}
	}
}

type confidentialFailingBody struct{ err error }

func (b confidentialFailingBody) Read([]byte) (int, error) { return 0, b.err }
func (confidentialFailingBody) Close() error               { return nil }

type confidentialFailingDecoder struct{ err error }

func (d confidentialFailingDecoder) UnmarshalJSON([]byte) error { return d.err }

type confidentialFailingEncoder struct{ err error }

func (e confidentialFailingEncoder) MarshalJSON() ([]byte, error) { return nil, e.err }

func TestConfidentialClientSanitizesBeforeTracingAndDropsPrivateErrorCause(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := &confidentialTraceRecorder{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	private := []string{"other-cpf-12345678901", "other-account@example.invalid", "raw-network-private", "raw-json-private", "raw-read-private", "raw-marshal-private"}
	for _, test := range []struct {
		name      string
		body      any
		dest      any
		transport roundTripFunc
	}{
		{
			name: "transport",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("raw-network-private")
			},
		},
		{
			name: "decode",
			dest: &confidentialFailingDecoder{err: errors.New("raw-json-private")},
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"other-account@example.invalid":"other-cpf-12345678901"}`))}, nil
			},
		},
		{
			name: "read",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: confidentialFailingBody{err: errors.New("raw-read-private")}}, nil
			},
		},
		{
			name: "marshal",
			body: confidentialFailingEncoder{err: errors.New("raw-marshal-private")},
			transport: func(*http.Request) (*http.Response, error) {
				t.Fatal("marshal failure reached provider transport")
				return nil, nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: test.transport}}
			err := DoRaw(WithConfidentialDiagnostics(context.Background()), c, http.MethodGet, "/v2/webhook/other-account@example.invalid?ref=other-cpf-12345678901", test.body, test.dest)
			if !errors.Is(err, ErrConfidentialRequest) || errors.Unwrap(err) != nil {
				t.Fatalf("confidential error did not terminate its private cause chain: %T", err)
			}
			for _, value := range private {
				if strings.Contains(err.Error(), value) {
					t.Fatal("private diagnostic reached returned error")
				}
			}
		})
	}
	recorder.assertNoPrivateDiagnostics(t, private)
}

func TestConfidentialClientPreservesOnlyHTTPStatusForExistingFallback(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error":"other-account@example.invalid","unknown":"other-cpf-12345678901"}`))}, nil
			})}}
			err := DoRaw(WithConfidentialDiagnostics(context.Background()), c, http.MethodGet, "/v2/webhook/private", nil, nil)
			var apiErr *EfiAPIError
			if !errors.As(err, &apiErr) || apiErr.Status != status || apiErr.Name != "provider_error" || apiErr.Message != http.StatusText(status) {
				t.Fatalf("HTTP status classification lost: %T", err)
			}
			if strings.Contains(err.Error(), "other-account") || strings.Contains(err.Error(), "other-cpf") || errors.Unwrap(err) != nil {
				t.Fatal("provider detail crossed the confidential boundary")
			}
		})
	}
}

func TestConfidentialClientRejectsCompleteOversizeBodyBeforeDecode(t *testing.T) {
	for _, size := range []int{MaxConfidentialResponseBytes, MaxConfidentialResponseBytes + 1} {
		raw := "{}" + strings.Repeat(" ", size-2)
		c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})}}
		var result map[string]any
		err := DoRaw(WithConfidentialDiagnostics(context.Background()), c, http.MethodGet, "/v2/webhook", nil, &result)
		if size == MaxConfidentialResponseBytes {
			if err != nil || result == nil {
				t.Fatalf("complete in-boundary response failed: %v", err)
			}
		} else if !errors.Is(err, ErrConfidentialResponseTooLarge) || result != nil {
			t.Fatal("oversize response was truncated or decoded as valid evidence")
		}
	}
}

func TestConfidentialMutationGuardRefusesMissingExpiredAndCanceledSends(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() context.Context
	}{
		{"missing", func() context.Context { return WithConfidentialDiagnostics(context.Background()) }},
		{"expired", func() context.Context {
			return WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return errors.New("private expired authorization") })
		}},
		{"canceled", func() context.Context {
			ctx, cancel := context.WithCancel(WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return nil }))
			cancel()
			return ctx
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}}
			err := DoRaw(test.ctx(), c, http.MethodPut, "/v2/webhook/private", map[string]any{"webhookUrl": "https://receiver.invalid"}, nil)
			if !errors.Is(err, ErrConfidentialMutationRefused) || calls != 0 || strings.Contains(err.Error(), "private") {
				t.Fatalf("refused PUT reached transport or disclosed reason: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestConfidentialExpiredGuardDoesNotBlockReadOnlyConfirmation(t *testing.T) {
	guardCalls, sends := 0, 0
	ctx := WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error {
		guardCalls++
		return errors.New("expired")
	})
	c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if r.Method != http.MethodGet {
			t.Fatal("expected read-only confirmation")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	if err := DoRaw(ctx, c, http.MethodGet, "/v2/webhook/private", nil, nil); err != nil || sends != 1 || guardCalls != 0 {
		t.Fatalf("expired mutation guard blocked read-only confirmation: sends=%d guards=%d err=%v", sends, guardCalls, err)
	}
}

func TestConfidentialOAuthHonorsCanceledContextWithoutPrivateErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "private-token"})
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(WithConfidentialDiagnostics(context.Background()))
	cancel()
	_, err := NewEfiClientWithContext(ctx, config.Config{ClientKeyID: "private-client", ClientSecret: "private-secret", BaseURL: srv.URL}, nil)
	if !errors.Is(err, ErrConfidentialRequest) || calls.Load() != 0 || errors.Unwrap(err) != nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("canceled confidential OAuth escaped: calls=%d err=%v", calls.Load(), err)
	}
}

func TestConfidentialOAuthUsesSameCompleteResponseLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"private-token"}`+strings.Repeat(" ", MaxConfidentialResponseBytes))
	}))
	defer srv.Close()
	_, err := NewEfiClientWithContext(WithConfidentialDiagnostics(context.Background()), config.Config{BaseURL: srv.URL}, nil)
	if !errors.Is(err, ErrConfidentialResponseTooLarge) || errors.Unwrap(err) != nil || strings.Contains(err.Error(), "private-token") {
		t.Fatal("oversize confidential OAuth was truncated into a token or leaked")
	}
}

func TestConfidentialAcceptedMutationRetainsSafeMarkerAfterHeaderSuccess(t *testing.T) {
	for _, test := range []struct {
		name string
		body func() io.ReadCloser
		dest any
	}{
		{"read-error", func() io.ReadCloser { return confidentialFailingBody{err: errors.New("private-post-put-read-cause")} }, nil},
		{"oversize", func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(strings.Repeat("x", MaxConfidentialResponseBytes+1)))
		}, nil},
		{"decode-error", func() io.ReadCloser { return io.NopCloser(strings.NewReader(`{}`)) }, &confidentialFailingDecoder{err: errors.New("private-post-put-decode-cause")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return nil })
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusAccepted, Body: test.body()}, nil
			})}}
			err := DoRaw(ctx, c, http.MethodPut, "/v2/webhook/private", nil, test.dest)
			if !MutationSucceeded(err) || !MutationSucceeded(fmt.Errorf("safe capability prefix: %w", err)) || !errors.Is(err, ErrConfidentialMutationSucceeded) {
				t.Fatal("known accepted PUT was lost behind a post-header failure")
			}
			if errors.Unwrap(err) != nil || strings.Contains(err.Error(), "private") {
				t.Fatal("safe accepted-mutation marker retained private post-PUT diagnostics")
			}
		})
	}
}

func TestAcceptedMutationMarkerDoesNotInventSuccessForGETNon2xxOrLegacy(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
		status int
		ctx    context.Context
	}{
		{"read-only-confirmation", http.MethodGet, http.StatusOK, WithConfidentialDiagnostics(context.Background())},
		{"rejected-mutation", http.MethodPut, http.StatusForbidden, WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return nil })},
		{"legacy-unchanged", http.MethodPut, http.StatusOK, context.Background()},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: confidentialFailingBody{err: errors.New("private-failure")}}, nil
			})}}
			err := DoRaw(test.ctx, c, test.method, "/v2/webhook/private", nil, nil)
			if err == nil || MutationSucceeded(err) {
				t.Fatal("post-header failure invented an accepted confidential PUT")
			}
		})
	}
	if MutationSucceeded(nil) || MutationSucceeded(errors.New("unrelated error")) {
		t.Fatal("accepted-mutation marker classified an unrelated outcome")
	}
}

func TestConfidentialPUTReceivesChildDeadlineWithoutCancelingOriginalContext(t *testing.T) {
	type originalValueKey struct{}
	parentDeadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(context.WithValue(context.Background(), originalValueKey{}, "retained"), parentDeadline)
	defer cancelParent()
	const remaining = 90 * time.Second
	ctx := WithMutationTimeout(WithMutationGuard(WithConfidentialDiagnostics(parent), func() error { return nil }), func() time.Duration { return remaining })
	var requestContext context.Context
	c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestContext = r.Context()
		deadline, ok := requestContext.Deadline()
		left := time.Until(deadline)
		if !ok || !deadline.Before(parentDeadline) || left <= 0 || left > remaining || requestContext.Value(originalValueKey{}) != "retained" {
			t.Fatal("PUT did not receive its bounded inherited child context")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	if err := DoRaw(ctx, c, http.MethodPut, "/v2/webhook/private", nil, nil); err != nil {
		t.Fatal(err)
	}
	if requestContext == nil || !errors.Is(requestContext.Err(), context.Canceled) || parent.Err() != nil || ctx.Err() != nil {
		t.Fatal("PUT child was not released or its cancellation escaped to original context")
	}
}

func TestConfidentialPUTRefusesExpiredMalformedOrNilTimeoutBeforeHTTP(t *testing.T) {
	for _, test := range []struct {
		name      string
		remaining func() time.Duration
	}{
		{"zero", func() time.Duration { return 0 }},
		{"negative", func() time.Duration { return -time.Nanosecond }},
		{"over-source-bound", func() time.Duration { return 5*time.Minute + 30*time.Second + time.Nanosecond }},
		{"nil", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			ctx := WithMutationTimeout(WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return nil }), test.remaining)
			c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}}
			err := DoRaw(ctx, c, http.MethodPut, "/v2/webhook/private", nil, nil)
			if !errors.Is(err, ErrConfidentialMutationRefused) || MutationSucceeded(err) || calls != 0 {
				t.Fatalf("invalid PUT timeout reached HTTP or invented success: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestConfidentialPUTDeadlineBodyFailureRetainsAcceptedMutationMarker(t *testing.T) {
	ctx := WithMutationTimeout(WithMutationGuard(WithConfidentialDiagnostics(context.Background()), func() error { return nil }), func() time.Duration { return time.Minute })
	var requestContext context.Context
	c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestContext = r.Context()
		if _, ok := requestContext.Deadline(); !ok {
			t.Fatal("PUT transport did not receive an authorization deadline")
		}
		// The actual body read happens after these accepted headers. Return
		// the HTTP body deadline failure deterministically; no timing sleep or
		// claim of a real provider execution is part of this fixture.
		return &http.Response{StatusCode: http.StatusOK, Body: confidentialFailingBody{err: context.DeadlineExceeded}}, nil
	})}}
	err := DoRaw(ctx, c, http.MethodPut, "/v2/webhook/private", nil, nil)
	if requestContext == nil || !MutationSucceeded(err) || !errors.Is(err, ErrConfidentialMutationSucceeded) || errors.Unwrap(err) != nil || ctx.Err() != nil {
		t.Fatal("accepted PUT deadline read failure lost safe event marker or canceled original context")
	}
}

func TestConfidentialGETIgnoresExpiredMutationTimeoutAndRetainsOriginalDeadline(t *testing.T) {
	type originalValueKey struct{}
	parentDeadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(context.WithValue(context.Background(), originalValueKey{}, "retained"), parentDeadline)
	defer cancelParent()
	guards, timeoutCalls, sends := 0, 0, 0
	ctx := WithMutationTimeout(WithMutationGuard(WithConfidentialDiagnostics(parent), func() error {
		guards++
		return errors.New("expired")
	}), func() time.Duration {
		timeoutCalls++
		return 0
	})
	c := &EfiClient{cfg: config.Config{BaseURL: "https://provider.invalid"}, httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		deadline, ok := r.Context().Deadline()
		if !ok || !deadline.Equal(parentDeadline) || r.Context().Value(originalValueKey{}) != "retained" {
			t.Fatal("read-only confirmation acquired mutation deadline or lost original context")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	if err := DoRaw(ctx, c, http.MethodGet, "/v2/webhook/private", nil, nil); err != nil || guards != 0 || timeoutCalls != 0 || sends != 1 || parent.Err() != nil || ctx.Err() != nil {
		t.Fatalf("expired mutation authorization blocked original read-only context: guards=%d timeouts=%d sends=%d err=%v", guards, timeoutCalls, sends, err)
	}
}
