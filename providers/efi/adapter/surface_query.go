package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// onSurfaceQuery is the read-only dispatcher behind the on_surface_query
// execute op. Core's /api/v1/integrations/{instance_id}/surface-query proxy
// hands it { query_name, params } as Input; it routes by query_name to a
// provider-specific read aggregator that returns a JSON shape the EFI/Pix
// finance-ops console surface renders directly. New surface tabs add new
// branches here; the surface read contract is the union of query_name
// strings accepted.
//
// Both wired reads are GET projections over the existing observe_* handlers
// (same EfiClient, same /oauth/token → Bearer GET seam) re-shaped for the
// surface. Read-only — they never mutate EFI state and move no money.
//
// rule #0 (payer-data minimization — HARDEST here): EFI's observe_charges
// returns the raw BCB payload, which carries `devedor` (payer nome / cpf /
// cnpj / email) and the pix legs (endToEndId). The surface read MUST NOT
// pass that through. surfaceCharges re-projects each charge to ONLY opaque
// refs — txid, valor, status, tipo, created — and DROPS everything else.
// This is the contract's forbidden "pay your bill" example: payer identity
// stays ops-internal, never on the operator surface.
//
// needs-work: reconciliation drift (EFI charges vs identities.webhook_event_efi
// — the killer feature for a finance-ops operator) is NOT wired here. It
// requires a cross-system join: the EFI side is readable here, but the
// identities.webhook_event_efi side lives in dakasa-system's enterprise DB
// and is only reachable through yggdrasil-core, not from this leaf adapter.
// Surfacing it means a core-side join (adapter read + core event-store read),
// not a single EFI GET. Left unwired rather than faked with a partial view.
//
// needs-work: payout / pró-labore history. There is no observe_payouts read
// op on this adapter — create_payout (PUT /v3/gn/pix/{idEnvio}) is write-only
// money-movement (IntermediateIrreversible), out of scope for a read surface.
// EFI also exposes no list-envios endpoint the adapter wraps today. Money-
// movement stays OUT of the reads; do not stub a payout history here.
func onSurfaceQuery(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	queryName := strings.TrimSpace(stringFromInput(in, "query_name"))
	if queryName == "" {
		return nil, fmt.Errorf("query_name is required")
	}
	params := asAnyMap(in["params"])

	switch queryName {
	case "list-webhook-subscriptions":
		out, err := surfaceWebhookSubscriptions(ctx, c, params)
		if err != nil {
			return nil, fmt.Errorf("list-webhook-subscriptions: %w", err)
		}
		return out, nil

	case "list-charges":
		out, err := surfaceCharges(ctx, c, params)
		if err != nil {
			return nil, fmt.Errorf("list-charges: %w", err)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("unknown query: %q", queryName)
	}
}

// surfaceWebhookSubscriptions is the headline pillar — the mTLS-hardened
// webhook subscription (Sec#2). It delegates to observe_webhook_subscriptions
// (GET /v2/webhook list, or GET /v2/webhook/{chave} when params.chave is set)
// and projects each subscription to {chave, url, status, mtls}.
//
//   - status: EFI's webhook API has no explicit per-subscription status field
//     (a subscription either exists or is absent), so a present subscription
//     is reported "active".
//   - mtls: EFI enforces mTLS on webhook delivery unless the subscription was
//     registered with the skip-mtls escape hatch. When upstream returns an
//     explicit skip indicator (skipMtls / mtls), we reflect it; otherwise we
//     report true (the hardened default this surface exists to highlight).
func surfaceWebhookSubscriptions(ctx context.Context, c *efiapi.EfiClient, params map[string]any) (map[string]any, error) {
	in := map[string]any{}
	if chave := strings.TrimSpace(stringFromInput(params, "chave")); chave != "" {
		in["chave"] = chave
	}
	raw, err := capabilities.ObserveWebhookSubscriptions(ctx, c, in)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	if list, ok := raw["webhooks"].([]any); ok {
		// list shape: {webhooks: [...]}
		for _, entry := range list {
			if m, ok := entry.(map[string]any); ok {
				rows = append(rows, projectWebhookSubscription(m))
			}
		}
	} else if _, hasChave := raw["chave"]; hasChave {
		// single-chave lookup returns the subscription object at top level.
		rows = append(rows, projectWebhookSubscription(raw))
	}

	if rows == nil {
		rows = []map[string]any{}
	}
	return map[string]any{"items": rows}, nil
}

// projectWebhookSubscription maps one raw BCB webhook subscription to the
// opaque surface shape {chave, url, status, mtls}. The webhook URL is an
// operator-owned endpoint (not payer PII), so it is safe to project.
func projectWebhookSubscription(m map[string]any) map[string]any {
	row := map[string]any{
		"chave":  stringFromInput(m, "chave"),
		"url":    firstNonEmptyString(m, "webhookUrl", "url"),
		"status": "active",
		"mtls":   true,
	}
	// Reflect an explicit upstream mTLS / skip-mTLS indicator when present.
	if skip, ok := boolFromInput(m, "skipMtls"); ok {
		row["mtls"] = !skip
	} else if skip, ok := boolFromInput(m, "skip_mtls"); ok {
		row["mtls"] = !skip
	} else if v, ok := boolFromInput(m, "mtls"); ok {
		row["mtls"] = v
	}
	return row
}

// surfaceCharges lists recent Pix charges for reconciliation context. It
// delegates to observe_charges — params carry either {txid} (single GET
// /v2/cob/{txid}) or {inicio, fim[, status, page, page_size]} (the statement
// window GET /v2/cob?...). The observe op's own "{txid} OR {inicio,fim}"
// contract is preserved: an empty filter surfaces its error (wrapped under
// the query name by the dispatcher).
//
// rule #0: the projection keeps ONLY opaque refs — txid, valor, status,
// tipo, created. The raw charge's devedor (payer nome/cpf/cnpj/email) and
// pix legs (endToEndId) are DROPPED, never projected.
func surfaceCharges(ctx context.Context, c *efiapi.EfiClient, params map[string]any) (map[string]any, error) {
	in := map[string]any{}
	if txid := strings.TrimSpace(stringFromInput(params, "txid")); txid != "" {
		in["txid"] = txid
	}
	if inicio := strings.TrimSpace(stringFromInput(params, "inicio")); inicio != "" {
		in["inicio"] = inicio
	}
	if fim := strings.TrimSpace(stringFromInput(params, "fim")); fim != "" {
		in["fim"] = fim
	}
	if status := strings.TrimSpace(stringFromInput(params, "status")); status != "" {
		in["status"] = status
	}
	if v, ok := params["page"]; ok {
		in["page"] = v
	}
	if v, ok := params["page_size"]; ok {
		in["page_size"] = v
	}

	raw, err := capabilities.ObserveCharges(ctx, c, in)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	switch {
	case rawHasChargeArray(raw, "cobs"):
		rows = projectChargeArray(raw["cobs"])
	case rawHasChargeArray(raw, "cobvs"):
		rows = projectChargeArray(raw["cobvs"])
	case raw["txid"] != nil:
		// single-charge lookup returns the cob/cobv object at top level.
		rows = []map[string]any{projectCharge(raw)}
	}

	if rows == nil {
		rows = []map[string]any{}
	}
	return map[string]any{"items": rows}, nil
}

// rawHasChargeArray reports whether raw[key] is a non-nil []any.
func rawHasChargeArray(raw map[string]any, key string) bool {
	arr, ok := raw[key].([]any)
	return ok && arr != nil
}

// projectChargeArray projects each entry of a cobs/cobvs array.
func projectChargeArray(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, entry := range arr {
		if m, ok := entry.(map[string]any); ok {
			out = append(out, projectCharge(m))
		}
	}
	return out
}

// projectCharge maps one raw BCB charge (cob or cobv) to the opaque surface
// shape {txid, valor, status, tipo, created}. This is the rule #0 boundary:
// NOTHING else from the upstream object — in particular `devedor`, `pix`,
// `infoAdicionais` — crosses into the projection.
func projectCharge(m map[string]any) map[string]any {
	calendario, _ := m["calendario"].(map[string]any)

	tipo := "cob"
	if calendario != nil {
		// A cobv (due charge) carries a due date in its calendario; an
		// immediate cob does not. This distinguishes the two without
		// trusting a caller-supplied hint.
		if _, due := calendario["dataDeVencimento"]; due {
			tipo = "cobv"
		} else if _, due := calendario["vencimento"]; due {
			tipo = "cobv"
		}
	}

	var created string
	if calendario != nil {
		created = stringFromInput(calendario, "criacao")
	}

	return map[string]any{
		"txid":    stringFromInput(m, "txid"),
		"valor":   chargeValor(m),
		"status":  stringFromInput(m, "status"),
		"tipo":    tipo,
		"created": created,
	}
}

// chargeValor extracts the charge amount as EFI returns it. BCB Pix nests it
// as valor.original (a decimal string, e.g. "10.00"); a few flows return a
// bare scalar. Returns "" when absent. The amount is not payer PII — it is an
// opaque reconciliation ref.
func chargeValor(m map[string]any) any {
	switch v := m["valor"].(type) {
	case map[string]any:
		if orig, ok := v["original"]; ok {
			return orig
		}
		return ""
	case nil:
		return ""
	default:
		return v
	}
}

// stringFromInput returns m[key] as a string ("" when absent or not a string).
func stringFromInput(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// firstNonEmptyString returns the first key whose value is a non-empty string.
func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := stringFromInput(m, k); s != "" {
			return s
		}
	}
	return ""
}

// boolFromInput coerces m[key] to bool; the second return is false when the
// key is absent or not a bool, so callers can distinguish "unset" from false.
func boolFromInput(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	if b, ok := m[key].(bool); ok {
		return b, true
	}
	return false, false
}

// asAnyMap coerces an input value into a map[string]any (params bag).
func asAnyMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
