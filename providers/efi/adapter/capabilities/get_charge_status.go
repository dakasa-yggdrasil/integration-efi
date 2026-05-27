package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// GetChargeStatus GETs /v2/cob/{txid} per BCB API. Returns the charge
// status + the pix[] array of completed transactions.
//
// Required input: txid.
//
// Idempotent — multiple calls return the same payload.
func GetChargeStatus(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	txid, _ := in["txid"].(string)
	if txid == "" {
		return nil, fmt.Errorf("get_charge_status: txid is required")
	}
	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/cob/"+txid, nil, &resp); err != nil {
		return nil, fmt.Errorf("get_charge_status: %w", err)
	}
	return resp, nil
}
