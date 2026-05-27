package capabilities

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// ObserveCharges is the v2.0.0 canonical read op for Pix charges. It
// collapses the v1.x get_charge_status + get_statement surfaces into a
// single resource_type-shaped capability matching the universal
// convention's observe_<resource_type> contract.
//
// Filter routing:
//   - {txid: X}          → GET /v2/cob/{txid} (single charge lookup; the
//                          v1.x get_charge_status path)
//   - {inicio, fim, ...} → GET /v2/cob?inicio=&fim=&page=&page_size=&status=
//                          (statement window; the v1.x get_statement path)
//
// Returns the upstream JSON payload verbatim, plus an envelope shape
// the SDK reconcile package can normalize (items[], cursor) once the
// adapter wires RegisterReconciler. For backward compat with v1.x
// callers, the raw upstream fields stay at the top level.
//
// Idempotent — read-only.
func ObserveCharges(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	if txid, _ := in["txid"].(string); txid != "" {
		return observeChargeByTxid(ctx, c, txid)
	}
	inicio, _ := in["inicio"].(string)
	fim, _ := in["fim"].(string)
	if inicio == "" || fim == "" {
		return nil, fmt.Errorf("observe_charges: requires either {txid} or {inicio, fim}")
	}
	return observeChargesByRange(ctx, c, in, inicio, fim)
}

// observeChargeByTxid wraps the v1.x get_charge_status BCB endpoint:
// GET /v2/cob/{txid}. Returns the upstream payload directly.
func observeChargeByTxid(ctx context.Context, c *efiapi.EfiClient, txid string) (map[string]any, error) {
	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/cob/"+txid, nil, &resp); err != nil {
		return nil, fmt.Errorf("observe_charges: %w", err)
	}
	return resp, nil
}

// observeChargesByRange wraps the v1.x get_statement BCB endpoint:
// GET /v2/cob with inicio/fim/status/page/page_size query params.
func observeChargesByRange(ctx context.Context, c *efiapi.EfiClient, in map[string]any, inicio, fim string) (map[string]any, error) {
	q := url.Values{}
	q.Set("inicio", inicio)
	q.Set("fim", fim)
	if status, _ := in["status"].(string); status != "" {
		q.Set("status", status)
	}
	if page, ok := numericInt(in["page"]); ok {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize, ok := numericInt(in["page_size"]); ok {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/cob?"+q.Encode(), nil, &resp); err != nil {
		return nil, fmt.Errorf("observe_charges: %w", err)
	}
	return resp, nil
}

// numericInt coerces an `any` payload value into an int. JSON
// unmarshalling produces float64 for numbers, so we coerce both float
// and int variants. Carried over from v1.x get_statement.go.
func numericInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		if x == "" {
			return 0, false
		}
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
