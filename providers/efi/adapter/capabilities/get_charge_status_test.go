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

func TestGetChargeStatus_GetsToCobAndReturnsStatusAndPix(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":   "tx-status-A",
			"status": "CONCLUIDA",
			"pix": []any{
				map[string]any{
					"endToEndId": "E2E-X",
					"valor":      "10.00",
				},
			},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := GetChargeStatus(context.Background(), c, map[string]any{"txid": "tx-status-A"})
	if err != nil {
		t.Fatalf("GetChargeStatus = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/cob/tx-status-A" {
		t.Fatalf("path = %q, want /v2/cob/tx-status-A", gotPath)
	}
	if got["status"] != "CONCLUIDA" {
		t.Fatalf("status = %v", got["status"])
	}
	if got["pix"] == nil {
		t.Fatalf("pix array missing from response")
	}
}

func TestGetChargeStatus_RequiresTxid(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := GetChargeStatus(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "txid") {
		t.Fatalf("expected txid required, got %v", err)
	}
}
