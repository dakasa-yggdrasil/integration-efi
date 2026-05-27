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

func TestDestroyWebhookSubscription_DeletesAndReturnsSuccess(t *testing.T) {
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
	got, err := DestroyWebhookSubscription(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("DestroyWebhookSubscription = %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v2/webhook/pix@dakasa.me" {
		t.Fatalf("path = %q", gotPath)
	}
	if got["destroyed"] != true {
		t.Fatalf("destroyed = %v", got["destroyed"])
	}
}

func TestDestroyWebhookSubscription_404IsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := DestroyWebhookSubscription(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("DestroyWebhookSubscription with 404 = %v, want nil (idempotent)", err)
	}
	if got["destroyed"] != true {
		t.Fatalf("destroyed = %v, want true even on 404", got["destroyed"])
	}
	if got["already_absent"] != true {
		t.Fatalf("already_absent = %v, want true on 404", got["already_absent"])
	}
}

func TestDestroyWebhookSubscription_RequiresChave(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := DestroyWebhookSubscription(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "chave") {
		t.Fatalf("expected chave required, got %v", err)
	}
}
