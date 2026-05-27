package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// DestroyCharge cancels (removes) a Pix charge by txid via the BCB
// Pix PATCH /v2/cob/{txid} contract, setting
// `status=REMOVIDA_PELO_USUARIO_RECEBEDOR`. v2.0.0 addition — pairs
// with ensure_charge / observe_charges to complete the canonical
// triple on the charge resource type.
//
// Required input: txid.
//
// Idempotent: 404 from EFI → success (the caller's desired state was
// "this charge should not exist," and it does not).
func DestroyCharge(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	txid, _ := in["txid"].(string)
	if txid == "" {
		return nil, fmt.Errorf("destroy_charge: txid is required")
	}
	body := map[string]any{
		"status": "REMOVIDA_PELO_USUARIO_RECEBEDOR",
	}
	err := efiapi.DoRaw(ctx, c, http.MethodPatch, "/v2/cob/"+txid, body, nil)
	if err == nil {
		return map[string]any{"destroyed": true, "txid": txid}, nil
	}
	var apiErr *efiapi.EfiAPIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return map[string]any{"destroyed": true, "txid": txid, "already_absent": true}, nil
	}
	return nil, fmt.Errorf("destroy_charge: %w", err)
}
