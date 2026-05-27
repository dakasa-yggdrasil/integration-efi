package capabilities

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestCreateCharge_PostsToCobWithBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":          "auto-tx-1234567890abcdefghij",
			"location":      "qr/auto-tx-1234567890abcdefghij",
			"pixCopiaECola": "00020126...",
			"status":        "ATIVA",
		})
	}))
	defer srv.Close()

	c, err := efiapi.NewEfiClient(config.Config{
		ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false,
	}, nil)
	if err != nil {
		t.Fatalf("NewEfiClient = %v", err)
	}

	got, err := CreateCharge(context.Background(), c, map[string]any{
		"valor": map[string]any{"original": "10.00"},
		"chave": "pix@dakasa.me",
	})
	if err != nil {
		t.Fatalf("CreateCharge = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if !strings.HasPrefix(gotPath, "/v2/cob") {
		t.Fatalf("path = %q, want /v2/cob*", gotPath)
	}
	if !strings.Contains(string(gotBody), `"original":"10.00"`) {
		t.Fatalf("body missing amount: %s", string(gotBody))
	}
	if !strings.Contains(string(gotBody), `"chave":"pix@dakasa.me"`) {
		t.Fatalf("body missing chave: %s", string(gotBody))
	}
	if got["txid"] != "auto-tx-1234567890abcdefghij" {
		t.Fatalf("txid = %v", got["txid"])
	}
	if got["status"] != "ATIVA" {
		t.Fatalf("status = %v", got["status"])
	}
}

func TestCreateCharge_WithCallerProvidedTxid_IsIdempotent(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":   "client-tx-XYZ",
			"status": "ATIVA",
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	_, err := CreateCharge(context.Background(), c, map[string]any{
		"valor": map[string]any{"original": "10.00"},
		"chave": "pix@dakasa.me",
		"txid":  "client-tx-XYZ",
	})
	if err != nil {
		t.Fatalf("CreateCharge = %v", err)
	}
	if gotPath != "/v2/cob/client-tx-XYZ" {
		t.Fatalf("path = %q, want /v2/cob/client-tx-XYZ", gotPath)
	}
}

func TestCreateCharge_RequiresValorAndChave(t *testing.T) {
	c := &efiapi.EfiClient{} // unused; validation runs before HTTP
	_, err := CreateCharge(context.Background(), c, map[string]any{"chave": "x"})
	if err == nil || !strings.Contains(err.Error(), "valor") {
		t.Fatalf("expected valor required, got %v", err)
	}
	_, err = CreateCharge(context.Background(), c, map[string]any{"valor": map[string]any{"original": "10.00"}})
	if err == nil || !strings.Contains(err.Error(), "chave") {
		t.Fatalf("expected chave required, got %v", err)
	}
}
