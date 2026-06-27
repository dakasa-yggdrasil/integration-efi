package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

// newSurfaceTestServer builds an httptest.Server that stubs the EFI
// /oauth/token handshake and delegates every other request to `h`. It
// mirrors the per-capability observe tests' seam: a real EfiClient is
// pointed at this server (mTLS disabled), so the surface reads exercise
// the same DoRaw → GET path the production observe_* handlers use.
func newSurfaceTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runSurfaceQuery invokes Execute(on_surface_query, {query_name, params})
// against an instance pointed at `srv` (mTLS off) and returns the Output
// bag. It asserts the call succeeded; per-query shape assertions live in
// the individual tests.
func runSurfaceQuery(t *testing.T, srv *httptest.Server, queryName string, params map[string]any) map[string]any {
	t.Helper()
	input := map[string]any{"query_name": queryName}
	if params != nil {
		input["params"] = params
	}
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: surfaceTestContext(srv),
		Input:       input,
	})
	if err != nil {
		t.Fatalf("Execute(on_surface_query, %s) = %v", queryName, err)
	}
	out, ok := resp.Output.(map[string]any)
	if !ok {
		t.Fatalf("Execute(on_surface_query, %s) Output is %T, want map[string]any", queryName, resp.Output)
	}
	return out
}

// surfaceTestContext builds the execute Integration context pointing a mock
// EFI instance at `srv` with mTLS disabled (so the OAuth + GET handshake
// runs against the httptest server without a client cert).
func surfaceTestContext(srv *httptest.Server) contract.AdapterExecuteIntegrationContext {
	return contract.AdapterExecuteIntegrationContext{
		InstanceSpec: contract.IntegrationInstanceManifestSpec{
			Credentials: map[string]any{"efi_client_key_id": "k", "efi_client_secret": "s"},
			Config:      map[string]any{"base_url": srv.URL, "mtls_enabled": false},
		},
	}
}

// TestOnSurfaceQuery_ListWebhookSubscriptions is the headline pillar — the
// mTLS-hardened webhook subscription (Sec#2). It projects
// observe_webhook_subscriptions list rows into {chave, url, status, mtls}.
func TestOnSurfaceQuery_ListWebhookSubscriptions(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/webhook" {
			t.Errorf("unexpected request %s %s, want GET /v2/webhook", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"webhooks": []any{
				map[string]any{"chave": "pix@dakasa.me", "webhookUrl": "https://webhook-h.dakasa.me/efi/webhook/pix", "criacao": "2026-05-27T00:00:00Z"},
				map[string]any{"chave": "pix2@dakasa.me", "webhookUrl": "https://webhook-h.dakasa.me/efi/webhook/pix2", "criacao": "2026-05-28T00:00:00Z"},
			},
		})
	})

	out := runSurfaceQuery(t, srv, "list-webhook-subscriptions", nil)
	items, ok := out["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items must be []map[string]any, got %T", out["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	first := items[0]
	if first["chave"] != "pix@dakasa.me" {
		t.Errorf("chave = %v, want pix@dakasa.me", first["chave"])
	}
	if first["url"] != "https://webhook-h.dakasa.me/efi/webhook/pix" {
		t.Errorf("url = %v", first["url"])
	}
	if first["status"] != "active" {
		t.Errorf("status = %v, want active", first["status"])
	}
	// mTLS reflects the headline pillar (Sec#2 hardened webhook). EFI's list
	// endpoint does not carry a per-row mtls flag, so it defaults to true
	// (the adapter enforces mTLS unless an instance opts out).
	if first["mtls"] != true {
		t.Errorf("mtls = %v, want true", first["mtls"])
	}
}

// TestOnSurfaceQuery_ListWebhookSubscriptions_RespectsMtlsField asserts that
// when EFI returns an explicit skip-mtls / mtls indicator on a subscription,
// the projection reflects it rather than blindly defaulting to true.
func TestOnSurfaceQuery_ListWebhookSubscriptions_RespectsMtlsField(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"webhooks": []any{
				map[string]any{"chave": "pix@dakasa.me", "webhookUrl": "https://a", "skipMtls": true},
			},
		})
	})

	out := runSurfaceQuery(t, srv, "list-webhook-subscriptions", nil)
	items := out["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0]["mtls"] != false {
		t.Errorf("mtls = %v, want false (skipMtls:true upstream)", items[0]["mtls"])
	}
}

// TestOnSurfaceQuery_ListCharges_StatementWindow projects an observe_charges
// statement window (GET /v2/cob?inicio=&fim=) into {txid, valor, status,
// tipo, created}. RULE #0: payer-identifying fields (devedor/nome/cpf) MUST
// NOT appear in the projection even though EFI returns them.
func TestOnSurfaceQuery_ListCharges_StatementWindow(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/cob" {
			t.Errorf("unexpected request %s %s, want GET /v2/cob", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("inicio"); !strings.HasPrefix(got, "2026-05-01") {
			t.Errorf("inicio = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"parametros": map[string]any{"inicio": "2026-05-01T00:00:00Z", "fim": "2026-05-31T23:59:59Z"},
			"cobs": []any{
				map[string]any{
					"txid":       "tx-A",
					"status":     "CONCLUIDA",
					"valor":      map[string]any{"original": "10.00"},
					"calendario": map[string]any{"criacao": "2026-05-10T12:00:00Z"},
					// PII the adapter must NOT project:
					"devedor": map[string]any{"nome": "Fulano de Tal", "cpf": "12345678909"},
				},
				map[string]any{
					"txid":       "tx-B",
					"status":     "ATIVA",
					"valor":      map[string]any{"original": "250.50"},
					"calendario": map[string]any{"criacao": "2026-05-11T08:30:00Z", "dataDeVencimento": "2026-06-11"},
					"devedor":    map[string]any{"nome": "Beltrana", "email": "beltrana@example.com"},
				},
			},
		})
	})

	out := runSurfaceQuery(t, srv, "list-charges", map[string]any{
		"inicio": "2026-05-01T00:00:00Z",
		"fim":    "2026-05-31T23:59:59Z",
	})
	items, ok := out["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items must be []map[string]any, got %T", out["items"])
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	first := items[0]
	if first["txid"] != "tx-A" {
		t.Errorf("txid = %v, want tx-A", first["txid"])
	}
	if first["valor"] != "10.00" {
		t.Errorf("valor = %v, want 10.00", first["valor"])
	}
	if first["status"] != "CONCLUIDA" {
		t.Errorf("status = %v, want CONCLUIDA", first["status"])
	}
	if first["tipo"] != "cob" {
		t.Errorf("tipo = %v, want cob", first["tipo"])
	}
	if first["created"] != "2026-05-10T12:00:00Z" {
		t.Errorf("created = %v", first["created"])
	}

	// A cobv-shaped charge (has dataDeVencimento) is classified tipo=cobv.
	second := items[1]
	if second["tipo"] != "cobv" {
		t.Errorf("tipo = %v, want cobv (has dataDeVencimento)", second["tipo"])
	}

	// RULE #0 — the absolute, contract-forbidden projection. No payer-
	// identifying key may leak through, on ANY row.
	forbidden := []string{"devedor", "nome", "cpf", "cnpj", "email", "pixKeyOfPayer", "pix_key_of_payer"}
	for i, item := range items {
		for _, k := range forbidden {
			if _, present := item[k]; present {
				t.Errorf("rule#0 VIOLATION: row %d projects forbidden payer key %q", i, k)
			}
		}
		// Defensive: ensure the only keys present are the allowlisted opaque refs.
		allowed := map[string]bool{"txid": true, "valor": true, "status": true, "tipo": true, "created": true}
		for k := range item {
			if !allowed[k] {
				t.Errorf("rule#0: row %d projects non-allowlisted key %q", i, k)
			}
		}
	}
}

// TestOnSurfaceQuery_ListCharges_ByTxid projects a single-charge lookup
// (GET /v2/cob/{txid}) into a one-row items[] with the same opaque-ref
// projection — and the same rule #0 PII suppression.
func TestOnSurfaceQuery_ListCharges_ByTxid(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/cob/tx-single" {
			t.Errorf("unexpected request %s %s, want GET /v2/cob/tx-single", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":       "tx-single",
			"status":     "CONCLUIDA",
			"valor":      map[string]any{"original": "42.00"},
			"calendario": map[string]any{"criacao": "2026-05-20T09:00:00Z"},
			"devedor":    map[string]any{"nome": "Sigiloso", "cpf": "00000000000"},
			"pix":        []any{map[string]any{"endToEndId": "E123", "valor": "42.00"}},
		})
	})

	out := runSurfaceQuery(t, srv, "list-charges", map[string]any{"txid": "tx-single"})
	items, ok := out["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items must be []map[string]any, got %T", out["items"])
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	row := items[0]
	if row["txid"] != "tx-single" || row["valor"] != "42.00" || row["status"] != "CONCLUIDA" || row["tipo"] != "cob" {
		t.Errorf("projection = %v", row)
	}
	if row["created"] != "2026-05-20T09:00:00Z" {
		t.Errorf("created = %v", row["created"])
	}
	// rule #0: no payer / pix-leg identifiers.
	for _, k := range []string{"devedor", "nome", "cpf", "pix", "endToEndId"} {
		if _, present := row[k]; present {
			t.Errorf("rule#0 VIOLATION: single-charge row projects forbidden key %q", k)
		}
	}
}

// TestOnSurfaceQuery_ChargeDetail projects the single-charge drill-down (GET
// /v2/cob/{txid}) into the detail shape {txid, valor, status, tipo, created,
// expiracao, devolucoes:[{id, valor, status, created}]}. It reuses the same
// observe_charges single-txid path as list-charges, but adds the expiracao and
// the flattened devoluções (refunds) the drill-down needs.
//
// RULE #0 (HARDEST here — EFI is the contract's FORBIDDEN "pay your bill"
// example): the upstream cob carries the payer `devedor` (nome / cpf / cnpj /
// email) and the pix legs (endToEndId / chave). NONE of that may appear in the
// projection — only opaque/operational refs. The devoluções are flattened from
// pix[].devolucoes[] WITHOUT carrying the endToEndId that nests them.
func TestOnSurfaceQuery_ChargeDetail(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/cob/tx-detail" {
			t.Errorf("unexpected request %s %s, want GET /v2/cob/tx-detail", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":   "tx-detail",
			"status": "CONCLUIDA",
			"valor":  map[string]any{"original": "150.00"},
			"calendario": map[string]any{
				"criacao":   "2026-05-10T12:00:00Z",
				"expiracao": 3600,
			},
			// PII the adapter must NOT project, on the charge or in the legs:
			"devedor": map[string]any{"nome": "Fulano de Tal", "cpf": "12345678909", "email": "fulano@example.com"},
			"pix": []any{
				map[string]any{
					"endToEndId": "E123ABC",
					"valor":      "150.00",
					"chave":      "pix@dakasa.me",
					"devolucoes": []any{
						map[string]any{
							"id":      "dev-1",
							"valor":   "50.00",
							"status":  "DEVOLVIDO",
							"horario": map[string]any{"solicitacao": "2026-05-11T09:00:00Z", "liquidacao": "2026-05-11T09:00:05Z"},
						},
						map[string]any{
							"id":      "dev-2",
							"valor":   "10.00",
							"status":  "EM_PROCESSAMENTO",
							"horario": map[string]any{"solicitacao": "2026-05-12T10:00:00Z"},
						},
					},
				},
			},
		})
	})

	out := runSurfaceQuery(t, srv, "charge-detail", map[string]any{"txid": "tx-detail"})

	if out["txid"] != "tx-detail" {
		t.Errorf("txid = %v, want tx-detail", out["txid"])
	}
	if out["valor"] != "150.00" {
		t.Errorf("valor = %v, want 150.00", out["valor"])
	}
	if out["status"] != "CONCLUIDA" {
		t.Errorf("status = %v, want CONCLUIDA", out["status"])
	}
	if out["tipo"] != "cob" {
		t.Errorf("tipo = %v, want cob", out["tipo"])
	}
	if out["created"] != "2026-05-10T12:00:00Z" {
		t.Errorf("created = %v, want 2026-05-10T12:00:00Z", out["created"])
	}
	// An immediate cob's expiracao is the validity window (seconds) from the
	// calendario — projected as-is (an opaque operational ref).
	if fmt.Sprintf("%v", out["expiracao"]) != "3600" {
		t.Errorf("expiracao = %v, want 3600", out["expiracao"])
	}

	devolucoes, ok := out["devolucoes"].([]map[string]any)
	if !ok {
		t.Fatalf("devolucoes must be []map[string]any, got %T", out["devolucoes"])
	}
	if len(devolucoes) != 2 {
		t.Fatalf("len(devolucoes) = %d, want 2", len(devolucoes))
	}
	if devolucoes[0]["id"] != "dev-1" || devolucoes[0]["valor"] != "50.00" || devolucoes[0]["status"] != "DEVOLVIDO" {
		t.Errorf("devolucoes[0] = %v", devolucoes[0])
	}
	if devolucoes[0]["created"] != "2026-05-11T09:00:00Z" {
		t.Errorf("devolucoes[0].created = %v, want 2026-05-11T09:00:00Z (horario.solicitacao)", devolucoes[0]["created"])
	}
	if devolucoes[1]["id"] != "dev-2" || devolucoes[1]["status"] != "EM_PROCESSAMENTO" {
		t.Errorf("devolucoes[1] = %v", devolucoes[1])
	}

	// RULE #0 — the charge object itself projects ONLY the allowlisted keys.
	// `provider` is the dispatcher's own metadata (the adapter's provider name,
	// "efi") that Execute seeds into every Output map — it is NOT payer data.
	allowed := map[string]bool{"provider": true, "txid": true, "valor": true, "status": true, "tipo": true, "created": true, "expiracao": true, "devolucoes": true}
	for k := range out {
		if !allowed[k] {
			t.Errorf("rule#0: charge-detail projects non-allowlisted key %q", k)
		}
	}
	forbidden := []string{"devedor", "nome", "cpf", "cnpj", "email", "pix", "pixKey", "chave", "endToEndId"}
	for _, k := range forbidden {
		if _, present := out[k]; present {
			t.Errorf("rule#0 VIOLATION: charge-detail projects forbidden payer/leg key %q", k)
		}
	}
	// RULE #0 — and no devolução row may carry an endToEndId / payer key either.
	devAllowed := map[string]bool{"id": true, "valor": true, "status": true, "created": true}
	for i, dev := range devolucoes {
		for k := range dev {
			if !devAllowed[k] {
				t.Errorf("rule#0: devolucoes[%d] projects non-allowlisted key %q", i, k)
			}
		}
		for _, k := range []string{"endToEndId", "chave", "devedor", "nome", "cpf", "horario"} {
			if _, present := dev[k]; present {
				t.Errorf("rule#0 VIOLATION: devolucoes[%d] projects forbidden key %q", i, k)
			}
		}
	}
}

// TestOnSurfaceQuery_ChargeDetail_CobvExpiracao asserts a due charge (cobv) is
// classified tipo=cobv and its expiracao comes from the dataDeVencimento — and
// that a charge with no pix legs yields an empty (non-nil) devolucoes list.
func TestOnSurfaceQuery_ChargeDetail_CobvExpiracao(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"txid":   "tx-cobv",
			"status": "ATIVA",
			"valor":  map[string]any{"original": "250.50"},
			"calendario": map[string]any{
				"criacao":          "2026-05-11T08:30:00Z",
				"dataDeVencimento": "2026-06-11",
			},
			"devedor": map[string]any{"nome": "Beltrana", "cnpj": "12345678000199"},
		})
	})

	out := runSurfaceQuery(t, srv, "charge-detail", map[string]any{"txid": "tx-cobv"})
	if out["tipo"] != "cobv" {
		t.Errorf("tipo = %v, want cobv (has dataDeVencimento)", out["tipo"])
	}
	if out["expiracao"] != "2026-06-11" {
		t.Errorf("expiracao = %v, want 2026-06-11 (dataDeVencimento)", out["expiracao"])
	}
	devolucoes, ok := out["devolucoes"].([]map[string]any)
	if !ok {
		t.Fatalf("devolucoes must be []map[string]any (non-nil), got %T", out["devolucoes"])
	}
	if len(devolucoes) != 0 {
		t.Errorf("len(devolucoes) = %d, want 0 (no pix legs)", len(devolucoes))
	}
	// rule #0 once more on the cobv path.
	for _, k := range []string{"devedor", "nome", "cnpj"} {
		if _, present := out[k]; present {
			t.Errorf("rule#0 VIOLATION: cobv charge-detail projects forbidden key %q", k)
		}
	}
}

// TestOnSurfaceQuery_ChargeDetail_RequiresTxid asserts the drill-down requires
// the txid param; the error is wrapped under the query name (an honest failure,
// not an empty object).
func TestOnSurfaceQuery_ChargeDetail_RequiresTxid(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not reach EFI when txid is missing; got %s %s", r.Method, r.URL.Path)
	})

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: surfaceTestContext(srv),
		Input:       map[string]any{"query_name": "charge-detail", "params": map[string]any{}},
	})
	if err == nil {
		t.Fatal("expected error when txid is missing")
	}
	if !strings.Contains(err.Error(), "charge-detail") || !strings.Contains(err.Error(), "txid") {
		t.Errorf("error must mention charge-detail and txid, got %v", err)
	}
}

// TestOnSurfaceQuery_ListCharges_RequiresFilter asserts the read surfaces
// observe_charges' own contract: a caller must supply {txid} OR {inicio,fim};
// the error is wrapped under the query name.
func TestOnSurfaceQuery_ListCharges_RequiresFilter(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not reach EFI when filter is missing; got %s %s", r.Method, r.URL.Path)
	})

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: surfaceTestContext(srv),
		Input:       map[string]any{"query_name": "list-charges", "params": map[string]any{}},
	})
	if err == nil {
		t.Fatal("expected error when no filter supplied")
	}
	if !strings.Contains(err.Error(), "list-charges") {
		t.Errorf("error must be wrapped under query name, got %v", err)
	}
}

// TestOnSurfaceQuery_UnknownQuery returns an error for an unrouted
// query_name so the surface gets an honest failure, not a silent empty.
func TestOnSurfaceQuery_UnknownQuery(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {})

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: surfaceTestContext(srv),
		Input:       map[string]any{"query_name": "reconciliation-drift"},
	})
	if err == nil {
		t.Fatal("expected error for unknown query")
	}
	if !strings.Contains(err.Error(), "unknown query") || !strings.Contains(err.Error(), "reconciliation-drift") {
		t.Errorf("error = %v, want 'unknown query: reconciliation-drift'", err)
	}
}

// TestOnSurfaceQuery_MissingQueryName errors when query_name is absent.
func TestOnSurfaceQuery_MissingQueryName(t *testing.T) {
	srv := newSurfaceTestServer(t, func(w http.ResponseWriter, r *http.Request) {})

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: surfaceTestContext(srv),
		Input:       map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error when query_name missing")
	}
	if !strings.Contains(err.Error(), "query_name") {
		t.Errorf("error = %v, want mention of query_name", err)
	}
}

// TestSpec_OnSurfaceQuery_InCatalogAsReactor pins the spec wiring: the op is
// in SupportedExecuteOperations, present in the ActionCatalog, and
// categorized as a reactor (on_ prefix → hidden from grant pickers).
func TestSpec_OnSurfaceQuery_InCatalogAsReactor(t *testing.T) {
	desc := Describe()
	var found *contract.IntegrationActionDefinition
	for i := range desc.ActionCatalog {
		if desc.ActionCatalog[i].Name == OperationOnSurfaceQuery {
			found = &desc.ActionCatalog[i]
			break
		}
	}
	if found == nil {
		t.Fatal("on_surface_query must be in ActionCatalog")
	}
	if found.Category != "reactor" {
		t.Errorf("Category = %q, want reactor (on_ prefix)", found.Category)
	}

	inSupported := false
	for _, op := range SupportedExecuteOperations {
		if op == OperationOnSurfaceQuery {
			inSupported = true
			break
		}
	}
	if !inSupported {
		t.Error("on_surface_query must be in SupportedExecuteOperations")
	}
}
