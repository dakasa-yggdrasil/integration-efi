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

func TestRefundCharge_PutsToPixDevolucaoWithBody(t *testing.T) {
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
			"id":      "dev-1",
			"rtrId":   "RTR-A",
			"valor":   "10.00",
			"status":  "DEVOLVIDO",
			"horario": map[string]string{"solicitacao": "2026-05-26T14:00:00Z"},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)

	got, err := RefundCharge(context.Background(), c, map[string]any{
		"e2eId": "E2E-abc",
		"id":    "dev-1",
		"valor": "10.00",
	})
	if err != nil {
		t.Fatalf("RefundCharge = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/v2/pix/E2E-abc/devolucao/dev-1" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), `"valor":"10.00"`) {
		t.Fatalf("body missing valor: %s", string(gotBody))
	}
	if got["status"] != "DEVOLVIDO" {
		t.Fatalf("status = %v", got["status"])
	}
}

func TestRefundCharge_RequiresE2eIdAndIdAndValor(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := RefundCharge(context.Background(), c, map[string]any{"id": "d", "valor": "10"})
	if err == nil || !strings.Contains(err.Error(), "e2eId") {
		t.Fatalf("expected e2eId required, got %v", err)
	}
	_, err = RefundCharge(context.Background(), c, map[string]any{"e2eId": "e", "valor": "10"})
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("expected id required, got %v", err)
	}
	_, err = RefundCharge(context.Background(), c, map[string]any{"e2eId": "e", "id": "d"})
	if err == nil || !strings.Contains(err.Error(), "valor") {
		t.Fatalf("expected valor required, got %v", err)
	}
}
