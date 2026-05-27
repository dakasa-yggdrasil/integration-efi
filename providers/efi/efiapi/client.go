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
// Pass nil (when EFI_MTLS_ENABLED=false) to disable mTLS — the OAuth
// call still happens but the server-cert chain is NOT validated.
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
		return nil, fmt.Errorf("efi: build oauth request: %w", err)
	}
	req.SetBasicAuth(cfg.ClientKeyID, cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("efi: oauth request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("efi: read oauth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &EfiAPIError{Status: resp.StatusCode, Name: "oauth_failed", Message: string(raw)}
	}

	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, fmt.Errorf("efi: decode oauth response: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("efi: oauth response had empty access_token")
	}

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
func (c *EfiClient) do(ctx context.Context, method, path string, body, dest any, headers map[string]string) error {
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
		return fmt.Errorf("efi: build request: %w", err)
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
		return fmt.Errorf("efi: do request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return fmt.Errorf("efi: read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &EfiAPIError{Status: resp.StatusCode, Name: resp.Status, Message: strings.TrimSpace(string(raw))}
	}
	if dest != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, dest); err != nil {
			return fmt.Errorf("efi: decode response: %w", err)
		}
	}
	return nil
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
