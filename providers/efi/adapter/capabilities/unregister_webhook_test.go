package capabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestUnregisterWebhookEndpoint_DeletesAndReturns204Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := UnregisterWebhookEndpoint(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("UnregisterWebhookEndpoint = %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v2/webhook/pix@dakasa.me" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["unregistered"] != true {
		t.Fatalf("unregistered = %v", got["unregistered"])
	}
}

func TestUnregisterWebhookEndpoint_404IsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := UnregisterWebhookEndpoint(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("UnregisterWebhookEndpoint with 404 = %v, want nil (idempotent)", err)
	}
	if got["unregistered"] != true {
		t.Fatalf("unregistered = %v, want true even on 404", got["unregistered"])
	}
}

func TestUnregisterWebhookEndpoint_RequiresChave(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := UnregisterWebhookEndpoint(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "chave") {
		t.Fatalf("expected chave required, got %v", err)
	}
}
