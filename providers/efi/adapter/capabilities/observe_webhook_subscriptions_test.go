package capabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
			"webhookUrl": "https://webhook.dakasa.me/efi/webhook",
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

func TestObserveWebhookSubscriptions_PathEscapesChave(t *testing.T) {
	const chave = "../private?token=must-not-be-query"
	var gotRequestURI string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotRequestURI = r.RequestURI
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{"chave": chave})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	if _, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{"chave": chave}); err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	wantRequestURI := "/v2/webhook/" + url.PathEscape(chave)
	if gotRequestURI != wantRequestURI {
		t.Fatalf("request URI = %q, want escaped %q", gotRequestURI, wantRequestURI)
	}
	if len(gotQuery) != 0 {
		t.Fatalf("chave injected provider query values: %v", gotQuery)
	}
}

func TestObserveWebhookSubscriptions_ByChaveRejectsMixedFilters(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{
		"chave":  "pix@dakasa.me",
		"inicio": "2026-08-01T00:00:00Z",
		"fim":    "2026-09-01T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mixed-filter validation error, got %v", err)
	}
}

// TestObserveWebhookSubscriptions_ByTimeRangeLists hits the list endpoint
// with the provider-required time range and BCB-native pagination query keys.
func TestObserveWebhookSubscriptions_ByTimeRangeLists(t *testing.T) {
	var gotMethod, gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"parametros": map[string]any{
				"inicio": "2026-08-01T00:00:00Z",
				"fim":    "2026-09-01T00:00:00Z",
				"paginacao": map[string]any{
					"paginaAtual":         0,
					"itensPorPagina":      50,
					"quantidadeDePaginas": 2,
				},
			},
			"webhooks": []any{
				map[string]any{"chave": "pix@dakasa.me", "webhookUrl": "https://a"},
				map[string]any{"chave": "pix2@dakasa.me", "webhookUrl": "https://b"},
			},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{
		"inicio":    "2026-08-01T00:00:00Z",
		"fim":       "2026-09-01T00:00:00Z",
		"page":      0,
		"page_size": 50,
	})
	if err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/webhook" {
		t.Fatalf("path = %q, want /v2/webhook", gotPath)
	}
	if gotQuery.Get("inicio") != "2026-08-01T00:00:00Z" || gotQuery.Get("fim") != "2026-09-01T00:00:00Z" {
		t.Fatalf("required range missing from query: %v", gotQuery)
	}
	if gotQuery.Get("paginacao.paginaAtual") != "0" {
		t.Fatalf("paginaAtual = %q, want 0", gotQuery.Get("paginacao.paginaAtual"))
	}
	if gotQuery.Get("paginacao.itensPorPagina") != "50" {
		t.Fatalf("itensPorPagina = %q, want 50", gotQuery.Get("paginacao.itensPorPagina"))
	}
	if gotQuery.Has("page") || gotQuery.Has("page_size") {
		t.Fatalf("adapter field names leaked into provider query: %v", gotQuery)
	}
	if got["webhooks"] == nil {
		t.Fatalf("webhooks array missing")
	}
	if got["cursor"] != "1" {
		t.Fatalf("cursor = %v, want next page 1", got["cursor"])
	}
}

func TestObserveWebhookSubscriptions_CursorRoutesToProviderPage(t *testing.T) {
	var gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotPage = r.URL.Query().Get("paginacao.paginaAtual")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"parametros": map[string]any{"paginacao": map[string]any{"paginaAtual": 1, "quantidadeDePaginas": 2}},
			"webhooks":   []any{},
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{
		"inicio": "2026-08-01T00:00:00Z",
		"fim":    "2026-09-01T00:00:00Z",
		"cursor": "1",
	})
	if err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	if gotPage != "1" {
		t.Fatalf("paginaAtual = %q, want cursor page 1", gotPage)
	}
	if got["cursor"] != "" {
		t.Fatalf("cursor = %v, want empty on final page", got["cursor"])
	}
}

func TestObserveWebhookSubscriptions_ListRequiresExplicitRange(t *testing.T) {
	c := &efiapi.EfiClient{}
	for _, in := range []map[string]any{
		{},
		{"inicio": "2026-08-01T00:00:00Z"},
		{"fim": "2026-09-01T00:00:00Z"},
	} {
		_, err := ObserveWebhookSubscriptions(context.Background(), c, in)
		if err == nil || !strings.Contains(err.Error(), "{inicio, fim}") {
			t.Fatalf("input %v: expected explicit range error, got %v", in, err)
		}
	}
}

func TestObserveWebhookSubscriptions_ValidatesTimeRange(t *testing.T) {
	c := &efiapi.EfiClient{}
	for _, tt := range []struct {
		name string
		in   map[string]any
		want string
	}{
		{name: "inicio type", in: map[string]any{"inicio": 1, "fim": "2026-09-01T00:00:00Z"}, want: "{inicio, fim}"},
		{name: "inicio syntax", in: map[string]any{"inicio": "2026-08-01", "fim": "2026-09-01T00:00:00Z"}, want: "inicio must be RFC3339"},
		{name: "fim syntax", in: map[string]any{"inicio": "2026-08-01T00:00:00Z", "fim": "tomorrow"}, want: "fim must be RFC3339"},
		{name: "reverse", in: map[string]any{"inicio": "2026-09-02T00:00:00Z", "fim": "2026-09-01T00:00:00Z"}, want: "greater than or equal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ObserveWebhookSubscriptions(context.Background(), c, tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestObserveWebhookSubscriptions_RedactsURLQuerySecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chave":      "pix@dakasa.me",
			"webhookUrl": "https://webhook.example/efi/webhook?hmac=must-not-escape&ignorar=",
		})
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := ObserveWebhookSubscriptions(context.Background(), c, map[string]any{"chave": "pix@dakasa.me"})
	if err != nil {
		t.Fatalf("ObserveWebhookSubscriptions = %v", err)
	}
	observedURL, _ := got["webhookUrl"].(string)
	if strings.Contains(observedURL, "must-not-escape") {
		t.Fatalf("observed webhook URL leaked query secret: %q", observedURL)
	}
	if !strings.Contains(observedURL, "hmac=") || !strings.Contains(observedURL, "ignorar=") {
		t.Fatalf("redacted URL lost query-key drift evidence: %q", observedURL)
	}
}

func TestObserveWebhookSubscriptions_RejectsInvalidPagination(t *testing.T) {
	c := &efiapi.EfiClient{}
	base := map[string]any{"inicio": "2026-08-01T00:00:00Z", "fim": "2026-09-01T00:00:00Z"}
	tests := []struct {
		name  string
		field string
		value any
	}{
		{name: "negative page", field: "page", value: -1},
		{name: "fractional page", field: "page", value: 1.5},
		{name: "zero page size", field: "page_size", value: 0},
		{name: "oversized page size", field: "page_size", value: 1001},
		{name: "invalid cursor", field: "cursor", value: "next"},
		{name: "non-string cursor", field: "cursor", value: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]any{"inicio": base["inicio"], "fim": base["fim"], tt.field: tt.value}
			if _, err := ObserveWebhookSubscriptions(context.Background(), c, in); err == nil {
				t.Fatalf("expected validation error for %s=%v", tt.field, tt.value)
			}
		})
	}

	in := map[string]any{"inicio": base["inicio"], "fim": base["fim"], "page": 1, "cursor": "2"}
	if _, err := ObserveWebhookSubscriptions(context.Background(), c, in); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected page/cursor conflict, got %v", err)
	}
}
