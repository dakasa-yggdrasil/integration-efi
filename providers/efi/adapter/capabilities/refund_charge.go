package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// RefundCharge PUTs /v2/pix/{e2eId}/devolucao/{id} per BCB API. The
// caller-provided `id` is what makes this idempotent — a repeated call
// with the same id returns the previously-recorded refund.
//
// Required input: e2eId, id, valor.
// Optional:       natureza, descricao.
func RefundCharge(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	e2eID, _ := in["e2eId"].(string)
	if e2eID == "" {
		return nil, fmt.Errorf("refund_charge: e2eId is required")
	}
	id, _ := in["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("refund_charge: id is required")
	}
	valor, _ := in["valor"].(string)
	if valor == "" {
		return nil, fmt.Errorf("refund_charge: valor is required")
	}

	body := map[string]any{"valor": valor}
	if v, ok := in["natureza"]; ok {
		body["natureza"] = v
	}
	if v, ok := in["descricao"]; ok {
		body["descricao"] = v
	}

	var resp map[string]any
	path := "/v2/pix/" + e2eID + "/devolucao/" + id
	if err := efiapi.DoRaw(ctx, c, http.MethodPut, path, body, &resp); err != nil {
		return nil, fmt.Errorf("refund_charge: %w", err)
	}
	return resp, nil
}
