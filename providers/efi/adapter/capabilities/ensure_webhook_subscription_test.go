package capabilities

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestEnsureWebhookSubscription_PutsToV2WithUrl(t *testing.T) {
	var gotMethod, gotPath, gotSkipMTLS string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSkipMTLS = r.Header.Get("x-skip-mtls-checking")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)

	got, err := EnsureWebhookSubscription(context.Background(), c, map[string]any{
		"chave":                "pix@dakasa.me",
		"webhook_url":          "https://webhook.dakasa.me/efi/webhook",
		"skip_mtls_validation": true,
	})
	if err != nil {
		t.Fatalf("EnsureWebhookSubscription = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/v2/webhook/pix@dakasa.me" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotSkipMTLS != "true" {
		t.Fatalf("x-skip-mtls-checking = %q, want true", gotSkipMTLS)
	}
	if !strings.Contains(string(gotBody), `"webhookUrl":"https://webhook.dakasa.me/efi/webhook"`) {
		t.Fatalf("body missing webhookUrl: %s", string(gotBody))
	}
	if got["ensured"] != true {
		t.Fatalf("ensured = %v", got["ensured"])
	}
}

func TestEnsureWebhookSubscription_RejectsFinalPixDeliveryPath(t *testing.T) {
	for _, webhookURL := range []string{
		"https://webhook.dakasa.me/efi/webhook/pix",
		"https://webhook.dakasa.me/efi/webhook/pix/",
	} {
		_, err := normalizeWebhookBaseURL(webhookURL)
		if err == nil || !strings.Contains(err.Error(), "EFI appends /pix") {
			t.Fatalf("webhook_url %q: expected appended-/pix validation error, got %v", webhookURL, err)
		}
	}
}

func TestEnsureWebhookSubscription_ValidatesSecureBaseURLWithoutEchoingIt(t *testing.T) {
	for _, webhookURL := range []string{
		"http://webhook.dakasa.me/efi/webhook",
		"https://user:password@webhook.dakasa.me/efi/webhook",
		"https://webhook.dakasa.me/efi/webhook#secret-fragment",
	} {
		_, err := normalizeWebhookBaseURL(webhookURL)
		if err == nil {
			t.Fatalf("webhook_url %q: expected validation error", webhookURL)
		}
		if strings.Contains(err.Error(), webhookURL) || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "secret-fragment") {
			t.Fatalf("validation error echoed sensitive URL material: %v", err)
		}
	}
}

func TestEnsureWebhookSubscription_AllowsDocumentedQueryFormOnBaseURL(t *testing.T) {
	want := "https://receiver.example/efi/webhook?hmac=opaque&ignorar="
	got, err := normalizeWebhookBaseURL(want)
	if err != nil {
		t.Fatalf("normalizeWebhookBaseURL() error = %v", err)
	}
	if got != want {
		t.Fatalf("normalized URL = %q, want %q", got, want)
	}
}

func TestEnsureWebhookSubscription_FallsBackToV3OnV2_404(t *testing.T) {
	var v2Calls, v3Calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v2/webhook/") {
			atomic.AddInt32(&v2Calls, 1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v3/gn/webhook/") {
			atomic.AddInt32(&v3Calls, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected path %q", r.URL.Path)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	_, err := EnsureWebhookSubscription(context.Background(), c, map[string]any{
		"chave":       "pix@dakasa.me",
		"webhook_url": "https://hook/",
	})
	if err != nil {
		t.Fatalf("EnsureWebhookSubscription = %v", err)
	}
	if v2Calls != 1 {
		t.Fatalf("v2 calls = %d, want 1", v2Calls)
	}
	if v3Calls != 1 {
		t.Fatalf("v3 fallback calls = %d, want 1", v3Calls)
	}
}

func TestEnsureWebhookSubscription_RequiresChaveAndUrl(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := EnsureWebhookSubscription(context.Background(), c, map[string]any{"webhook_url": "u"})
	if err == nil || !strings.Contains(err.Error(), "chave") {
		t.Fatalf("expected chave required, got %v", err)
	}
	_, err = EnsureWebhookSubscription(context.Background(), c, map[string]any{"chave": "c"})
	if err == nil || !strings.Contains(err.Error(), "webhook_url") {
		t.Fatalf("expected webhook_url required, got %v", err)
	}
}
