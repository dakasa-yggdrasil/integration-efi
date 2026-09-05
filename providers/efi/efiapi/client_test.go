package efiapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
)

func TestNewEfiClient_OAuthSuccess(t *testing.T) {
	var gotMethod, gotPath, gotAuthHeader string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuthHeader = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-abc-123"})
	}))
	defer srv.Close()

	cfg := config.Config{
		ClientKeyID:  "ckid-test",
		ClientSecret: "csec-test",
		BaseURL:      srv.URL,
		MTLSEnabled:  false,
	}
	c, err := NewEfiClient(cfg, &tls.Config{})
	if err != nil {
		t.Fatalf("NewEfiClient() error = %v", err)
	}
	if c.Token() != "tok-abc-123" {
		t.Fatalf("Token = %q, want tok-abc-123", c.Token())
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/oauth/token" {
		t.Fatalf("path = %q, want /oauth/token", gotPath)
	}
	if !strings.HasPrefix(gotAuthHeader, "Basic ") {
		t.Fatalf("auth header = %q, want Basic prefix", gotAuthHeader)
	}
	if !strings.Contains(string(gotBody), "client_credentials") {
		t.Fatalf("body = %s, want grant_type=client_credentials", string(gotBody))
	}
}

func TestNewEfiClient_OAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"name":"invalid_client","message":"client_secret=must-not-escape"}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		ClientKeyID:  "wrong",
		ClientSecret: "wrong",
		BaseURL:      srv.URL,
		MTLSEnabled:  false,
	}
	_, err := NewEfiClient(cfg, nil)
	if err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
	var apiErr *EfiAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *EfiAPIError", err)
	}
	if apiErr.Status != 401 {
		t.Fatalf("status = %d, want 401", apiErr.Status)
	}
	if strings.Contains(err.Error(), "must-not-escape") {
		t.Fatalf("OAuth error leaked provider-controlled credential detail: %v", err)
	}
	if apiErr.Message != "authentication failed" {
		t.Fatalf("message = %q, want static authentication failure", apiErr.Message)
	}
}

func TestEfiAPIError_DoesNotExposeProviderBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"nome":"webhook_invalido",
			"mensagem":"URL https://receiver.example/webhook?hmac=url-secret&ignorar= rejeitada; token=detail-secret; Authorization: Bearer bearer-secret",
			"debug":"unknown-field-secret"
		}`))
	}))
	defer srv.Close()

	c, err := NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL}, nil)
	if err != nil {
		t.Fatalf("NewEfiClient() error = %v", err)
	}
	err = DoRaw(context.Background(), c, http.MethodGet, "/v2/webhook/key", nil, nil)
	if err == nil {
		t.Fatal("expected provider error")
	}
	for _, secret := range []string{"webhook_invalido", "url-secret", "detail-secret", "bearer-secret", "unknown-field-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("provider error leaked %q: %v", secret, err)
		}
	}
	var apiErr *EfiAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *EfiAPIError", err)
	}
	if apiErr.Name != "provider_error" || apiErr.Message != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("unsafe or unexpected provider error: %#v", apiErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestEfiTransportError_DoesNotExposeRequestOrUnderlyingDetail(t *testing.T) {
	underlying := errors.New("dial failed with must-not-escape")
	c := &EfiClient{
		cfg: config.Config{BaseURL: "https://provider.invalid"},
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, underlying
		})},
		token: "internal-token",
	}
	err := DoRaw(context.Background(), c, http.MethodGet, "/v2/webhook/sensitive-key?token=query-secret", nil, nil)
	if err == nil {
		t.Fatal("expected transport error")
	}
	for _, sensitive := range []string{"sensitive-key", "query-secret", "must-not-escape", "internal-token"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("transport error leaked %q: %v", sensitive, err)
		}
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("transport error lost unwrap chain: %v", err)
	}
}

func TestEfiAPIError_TransientClassification(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusBadRequest, false},
		{http.StatusForbidden, false},
		{http.StatusUnprocessableEntity, false},
		{http.StatusInternalServerError, false}, // 500 deliberately non-transient — EFI's contract
		{http.StatusNotFound, false},
	}
	for _, tc := range cases {
		err := &EfiAPIError{Status: tc.status}
		if got := err.IsTransient(); got != tc.want {
			t.Errorf("status=%d IsTransient() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestIsTransientError_WithNonAPIError(t *testing.T) {
	if IsTransientError(errors.New("connection refused")) {
		t.Fatalf("plain error should not be transient (SDK already handles transport-level retries)")
	}
	if IsTransientError(nil) {
		t.Fatalf("nil error should not be transient")
	}
}
