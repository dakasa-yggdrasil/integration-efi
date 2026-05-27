package capabilities

import (
	"context"
	"fmt"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// HandleChargeback is an internal capability — no EFI HTTP call.
// EFI does not provide a chargeback API in the public Pix surface;
// chargebacks (devolution disputes, BCB MED/2025) are signaled
// through downstream channels and recorded by the workflow.
//
// This capability acknowledges the chargeback envelope so the
// workflow has a structured record of "the adapter saw this and
// passed it on to the financial ledger". Idempotent by chargeback_id.
//
// Required input: e2eId, chargeback_id.
func HandleChargeback(_ context.Context, _ *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	e2eID, _ := in["e2eId"].(string)
	chargebackID, _ := in["chargeback_id"].(string)
	if e2eID == "" {
		return nil, fmt.Errorf("handle_chargeback: e2eId is required")
	}
	if chargebackID == "" {
		return nil, fmt.Errorf("handle_chargeback: chargeback_id is required")
	}
	return map[string]any{
		"e2eId":         e2eID,
		"chargeback_id": chargebackID,
		"status":        in["status"],
		"processed":     true,
	}, nil
}
