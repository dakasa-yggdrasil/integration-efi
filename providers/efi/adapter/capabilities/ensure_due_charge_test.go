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

func TestEnsureDueCharge_PutsToCobvWithDataDeVencimento(t *testing.T) {
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
			"txid":   "due-tx-A",
			"status": "ATIVA",
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)

	got, err := EnsureDueCharge(context.Background(), c, map[string]any{
		"txid":  "due-tx-A",
		"valor": map[string]any{"original": "55.00"},
		"chave": "pix@dakasa.me",
		"calendario": map[string]any{
			"dataDeVencimento": "2026-06-15",
		},
		"devedor": map[string]any{
			"cpf":  "00011122233",
			"nome": "Test User",
		},
	})
	if err != nil {
		t.Fatalf("EnsureDueCharge = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/v2/cobv/due-tx-A" {
		t.Fatalf("path = %q, want /v2/cobv/due-tx-A", gotPath)
	}
	if !strings.Contains(string(gotBody), `"dataDeVencimento":"2026-06-15"`) {
		t.Fatalf("body missing dataDeVencimento: %s", string(gotBody))
	}
	if got["txid"] != "due-tx-A" {
		t.Fatalf("txid = %v", got["txid"])
	}
}

func TestEnsureDueCharge_RequiresAllFields(t *testing.T) {
	c := &efiapi.EfiClient{}

	_, err := EnsureDueCharge(context.Background(), c, map[string]any{
		"valor": map[string]any{"original": "10.00"},
		"chave": "x",
		"calendario": map[string]any{
			"dataDeVencimento": "2026-06-15",
		},
		"devedor": map[string]any{
			"cpf": "1", "nome": "n",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "txid") {
		t.Fatalf("expected txid required, got %v", err)
	}

	_, err = EnsureDueCharge(context.Background(), c, map[string]any{
		"txid":  "t",
		"valor": map[string]any{"original": "10.00"},
		"chave": "x",
		"devedor": map[string]any{
			"cpf": "1", "nome": "n",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "calendario.dataDeVencimento") {
		t.Fatalf("expected calendario.dataDeVencimento required, got %v", err)
	}
}
