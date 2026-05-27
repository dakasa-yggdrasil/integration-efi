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

func TestGetStatement_GetsCobWithQueryParams(t *testing.T) {
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
	got, err := GetStatement(context.Background(), c, map[string]any{
		"inicio":    "2026-05-01T00:00:00Z",
		"fim":       "2026-05-31T23:59:59Z",
		"page":      0,
		"page_size": 100,
	})
	if err != nil {
		t.Fatalf("GetStatement = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/cob" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "inicio=2026-05-01T00") {
		t.Fatalf("query missing inicio: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "fim=2026-05-31T23") {
		t.Fatalf("query missing fim: %s", gotQuery)
	}
	if got["parametros"] == nil {
		t.Fatalf("parametros missing")
	}
}

func TestGetStatement_RequiresInicioAndFim(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := GetStatement(context.Background(), c, map[string]any{"fim": "x"})
	if err == nil || !strings.Contains(err.Error(), "inicio") {
		t.Fatalf("expected inicio required, got %v", err)
	}
	_, err = GetStatement(context.Background(), c, map[string]any{"inicio": "x"})
	if err == nil || !strings.Contains(err.Error(), "fim") {
		t.Fatalf("expected fim required, got %v", err)
	}
}
