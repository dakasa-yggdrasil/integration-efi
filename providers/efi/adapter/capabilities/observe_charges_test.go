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

// TestObserveCharges_ByTxid_RoutesToCobLookup asserts the merged
// observe_charges hits the same GET /v2/cob/{txid} the v1.x
// get_charge_status used when filter carries {txid}.
func TestObserveCharges_ByTxid_RoutesToCobLookup(t *testing.T) {
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
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveCharges(context.Background(), c, map[string]any{"txid": "tx-status-A"})
	if err != nil {
		t.Fatalf("ObserveCharges by txid = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/cob/tx-status-A" {
		t.Fatalf("path = %q, want /v2/cob/tx-status-A", gotPath)
	}
	if got["status"] != "CONCLUIDA" {
		t.Fatalf("status = %v, want CONCLUIDA", got["status"])
	}
}

// TestObserveCharges_ByTimeRange_RoutesToStatement asserts the
// statement window path GET /v2/cob?inicio=&fim=& is hit when filter
// carries inicio+fim (the v1.x get_statement path).
func TestObserveCharges_ByTimeRange_RoutesToStatement(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"parametros": map[string]any{"inicio": "2026-05-01T00:00:00Z", "fim": "2026-05-31T23:59:59Z"},
			"cobs":       []any{},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveCharges(context.Background(), c, map[string]any{
		"inicio":    "2026-05-01T00:00:00Z",
		"fim":       "2026-05-31T23:59:59Z",
		"page":      0,
		"page_size": 100,
	})
	if err != nil {
		t.Fatalf("ObserveCharges by range = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/cob" {
		t.Fatalf("path = %q, want /v2/cob", gotPath)
	}
	if !strings.Contains(gotQuery, "inicio=2026-05-01T00") {
		t.Fatalf("query missing inicio: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "fim=2026-05-31T23") {
		t.Fatalf("query missing fim: %s", gotQuery)
	}
	if got["parametros"] == nil {
		t.Fatalf("parametros missing from statement response")
	}
}

// TestObserveCharges_NoFilterReturnsError documents the contract:
// the caller MUST supply either {txid} or {inicio, fim}. There is no
// "list everything" fallback.
func TestObserveCharges_NoFilterReturnsError(t *testing.T) {
	c := &efiapi.EfiClient{} // unused; validation runs before HTTP
	_, err := ObserveCharges(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "txid") || !strings.Contains(err.Error(), "inicio") {
		t.Fatalf("expected filter required error, got %v", err)
	}
}

// TestObserveCharges_OnlyInicioReturnsError asserts partial range is
// still rejected (fim is required when inicio is set).
func TestObserveCharges_OnlyInicioReturnsError(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := ObserveCharges(context.Background(), c, map[string]any{"inicio": "2026-05-01T00:00:00Z"})
	if err == nil {
		t.Fatal("expected error when fim is missing")
	}
}
