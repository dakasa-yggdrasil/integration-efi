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

func TestCreatePayout_PutsToGnPixWithBody(t *testing.T) {
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
			"idEnvio": "ENV-1",
			"e2eId":   "E2E-payout-A",
			"valor":   "10.00",
			"status":  "EM_PROCESSAMENTO",
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)

	got, err := CreatePayout(context.Background(), c, map[string]any{
		"idEnvio": "ENV-1",
		"valor":   "10.00",
		"pagador": map[string]any{"chave": "pagador@dakasa.me"},
		"favorecido": map[string]any{
			"chave": "favorecido@dakasa.me",
		},
	})
	if err != nil {
		t.Fatalf("CreatePayout = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/v3/gn/pix/ENV-1" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), `"chave":"pagador@dakasa.me"`) {
		t.Fatalf("body missing pagador chave: %s", string(gotBody))
	}
	if got["status"] != "EM_PROCESSAMENTO" {
		t.Fatalf("status = %v", got["status"])
	}
}

func TestCreatePayout_RequiresIdEnvioAndValorAndPagadorAndFavorecido(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := CreatePayout(context.Background(), c, map[string]any{
		"valor": "10.00", "pagador": map[string]any{"chave": "p"}, "favorecido": map[string]any{"chave": "f"},
	})
	if err == nil || !strings.Contains(err.Error(), "idEnvio") {
		t.Fatalf("expected idEnvio required, got %v", err)
	}
	_, err = CreatePayout(context.Background(), c, map[string]any{
		"idEnvio": "E", "pagador": map[string]any{"chave": "p"}, "favorecido": map[string]any{"chave": "f"},
	})
	if err == nil || !strings.Contains(err.Error(), "valor") {
		t.Fatalf("expected valor required, got %v", err)
	}
	_, err = CreatePayout(context.Background(), c, map[string]any{
		"idEnvio": "E", "valor": "10", "favorecido": map[string]any{"chave": "f"},
	})
	if err == nil || !strings.Contains(err.Error(), "pagador") {
		t.Fatalf("expected pagador.chave required, got %v", err)
	}
	_, err = CreatePayout(context.Background(), c, map[string]any{
		"idEnvio": "E", "valor": "10", "pagador": map[string]any{"chave": "p"},
	})
	if err == nil || !strings.Contains(err.Error(), "favorecido") {
		t.Fatalf("expected favorecido required, got %v", err)
	}
}
