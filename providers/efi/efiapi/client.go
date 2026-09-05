// Package efiapi hosts the HTTP client used by every capability to
// talk to EFI's API. It lives in a sibling package so that the
// capabilities subpackage can import it without creating an import
// cycle with the adapter package (which routes Execute() through both
// the client and the capabilities).
package efiapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
)

// EfiClient is the HTTP client used to talk to pix.api.efipay.com.br
// (and pix-h homologation) over mTLS, with OAuth Basic-auth →
// Bearer-token handoff.
type EfiClient struct {
	cfg        config.Config
	httpClient *http.Client
	token      string
}

// Token returns the cached OAuth bearer token.
func (c *EfiClient) Token() string { return c.token }

// NewEfiClient authenticates against EFI's /oauth/token using Basic
// auth (key_id/secret) over mTLS, caches the token, and returns the
// client.
//
// The supplied tlsConfig is used as the http.Transport.TLSClientConfig.
// Pass nil (when EFI_MTLS_ENABLED=false) to omit the client certificate. The
// default transport still validates the provider's server certificate against
// the host system roots.
func NewEfiClient(cfg config.Config, tlsConfig *tls.Config) (*EfiClient, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	body := strings.NewReader(`{"grant_type": "client_credentials"}`)
	req, err := http.NewRequest(http.MethodPost, cfg.BaseURL+"/oauth/token", body)
	if err != nil {
		return nil, errors.New("efi: build oauth request")
	}
	req.SetBasicAuth(cfg.ClientKeyID, cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &EfiTransportError{Operation: "oauth", Err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("efi: read oauth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := newEfiAPIError(resp.StatusCode, "oauth_failed")
		// OAuth responses are external and untrusted. Never forward their
		// free-form detail because it can reflect credential material into RPC
		// errors and logs.
		apiErr.Message = "authentication failed"
		return nil, apiErr
	}

	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &token); err != nil {
		OAuthRefreshes.WithLabelValues("decode_fail").Inc()
		return nil, fmt.Errorf("efi: decode oauth response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		OAuthRefreshes.WithLabelValues("empty_token").Inc()
		return nil, fmt.Errorf("efi: oauth response had empty access_token")
	}

	OAuthRefreshes.WithLabelValues("success").Inc()
	return &EfiClient{
		cfg:        cfg,
		httpClient: httpClient,
		token:      token.AccessToken,
	}, nil
}

// EfiAPIError is the structured error returned by every non-2xx EFI
// response. Callers classify transient vs terminal via IsTransient().
type EfiAPIError struct {
	Status  int    `json:"status"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (e *EfiAPIError) Error() string {
	return fmt.Sprintf("efi: (status=%d) %s: %s", e.Status, e.Name, e.Message)
}

// IsTransient returns true for HTTP statuses callers should retry.
// EFI's API contract is: 5xx (except 500 — deliberate non-transient)
// + 429 are transient. 4xx are terminal (caller bug).
func (e *EfiAPIError) IsTransient() bool {
	return e.Status == http.StatusTooManyRequests || e.Status == http.StatusServiceUnavailable || e.Status == http.StatusGatewayTimeout
}

// IsTransientError unwraps *EfiAPIError and checks IsTransient().
// Returns false on nil or non-API errors (transport-level errors are
// the SDK's responsibility to retry).
func IsTransientError(err error) bool {
	var apiErr *EfiAPIError
	if errors.As(err, &apiErr) {
		return apiErr.IsTransient()
	}
	return false
}

// do issues a Bearer-auth request and decodes the JSON response into
// dest. Returns *EfiAPIError on non-2xx so callers can classify.
// Records efi_request_duration_seconds + efi_request_errors_total
// AND emits an OpenTelemetry span (1 span per EFI API call).
func (c *EfiClient) do(ctx context.Context, method, path string, body, dest any, headers map[string]string) error {
	op := classifyPath(path)
	tracer := otel.Tracer("integration-efi")
	ctx, span := tracer.Start(ctx, "efi."+op)
	defer span.End()
	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("efi.operation", op),
	)

	start := time.Now()
	err := c.doInner(ctx, method, path, body, dest, headers)
	statusClass := "2xx"
	var apiErr *EfiAPIError
	if errors.As(err, &apiErr) {
		statusClass = fmt.Sprintf("%dxx", apiErr.Status/100)
		RequestErrors.WithLabelValues(op, fmt.Sprint(apiErr.Status)).Inc()
		span.SetAttributes(attribute.Int("http.status_code", apiErr.Status))
		span.RecordError(err)
	} else if err != nil {
		statusClass = "transport"
		span.RecordError(err)
	}
	RequestDuration.WithLabelValues(op, statusClass).Observe(time.Since(start).Seconds())
	return err
}

// doInner is the non-instrumented core; metric wrapping lives in do().
func (c *EfiClient) doInner(ctx context.Context, method, path string, body, dest any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("efi: marshal request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, reader)
	if err != nil {
		return errors.New("efi: build provider request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &EfiTransportError{Operation: classifyPath(path), Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return fmt.Errorf("efi: read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newEfiAPIError(resp.StatusCode, "provider_error")
	}
	if dest != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dest); err != nil {
			return fmt.Errorf("efi: decode response: %w", err)
		}
	}
	return nil
}

// EfiTransportError preserves the underlying network error for retry and
// errors.Is/errors.As decisions while keeping request URLs, Pix keys, query
// secrets, and HTTP-client diagnostics out of Error(), RPC responses, logs,
// and trace events.
type EfiTransportError struct {
	Operation string
	Err       error
}

func (e *EfiTransportError) Error() string {
	operation := sanitizeProviderText(e.Operation, 64)
	if operation == "" {
		operation = "provider"
	}
	return fmt.Sprintf("efi: %s transport request failed", operation)
}

func (e *EfiTransportError) Unwrap() error { return e.Err }

// newEfiAPIError intentionally does not forward the provider response body.
// Validation errors can reflect Pix keys, webhook URLs with HMAC query values,
// or credentials. No heuristic redactor can prove an arbitrary secret is absent,
// so only the HTTP status and a caller-owned static name cross the boundary.
func newEfiAPIError(status int, staticName string) *EfiAPIError {
	name := sanitizeProviderText(staticName, 96)
	message := http.StatusText(status)
	if name == "" {
		name = "provider_error"
	}
	if message == "" {
		message = "provider request failed"
	}
	return &EfiAPIError{Status: status, Name: name, Message: message}
}

func sanitizeProviderText(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "..."
	}
	return value
}

// DoRaw is exported so capability subpackages can issue authenticated
// requests without leaking *EfiClient internals.
func DoRaw(ctx context.Context, c *EfiClient, method, path string, body, dest any) error {
	return c.do(ctx, method, path, body, dest, nil)
}

// DoRawWithHeaders is DoRaw + caller-supplied extra request headers.
// Use for endpoints where EFI accepts non-standard headers such as
// `x-skip-mtls-checking` (register_webhook_endpoint).
func DoRawWithHeaders(ctx context.Context, c *EfiClient, method, path string, body, dest any, headers map[string]string) error {
	return c.do(ctx, method, path, body, dest, headers)
}
