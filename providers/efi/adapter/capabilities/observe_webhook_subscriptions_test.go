package capabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// TestObserveWebhookSubscriptions_ByChave hits the single-subscription
// lookup (GET /v2/webhook/{chave}) when filter.chave is set.
func TestObserveWebhookSubscriptions_ByChave(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chave":      "pix@dakasa.me",
			"webhookUrl": "https://webhook.dakasa.me/efi/webhook/pix",
			"criacao":    "2026-05-27T00:00:00Z",
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/webhook/pix@dakasa.me" {
		t.Fatalf("path = %q, want /v2/webhook/pix@dakasa.me", gotPath)
	}
	if got["chave"] != "pix@dakasa.me" {
		t.Fatalf("chave = %v", got["chave"])
	}
}

// TestObserveWebhookSubscriptions_NoFilterLists hits the list endpoint
// (GET /v2/webhook) when no filter is set.
func TestObserveWebhookSubscriptions_NoFilterLists(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"webhooks": []any{
				map[string]any{"chave": "pix@dakasa.me", "webhookUrl": "https://a"},
				map[string]any{"chave": "pix2@dakasa.me", "webhookUrl": "https://b"},
			},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{})
	if err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/webhook" {
		t.Fatalf("path = %q, want /v2/webhook", gotPath)
	}
	if got["webhooks"] == nil {
		t.Fatalf("webhooks array missing")
	}
}
