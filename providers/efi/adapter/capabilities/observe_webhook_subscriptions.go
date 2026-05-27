package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// ObserveWebhookSubscriptions implements the canonical observe_ side
// of the webhook_subscription resource. v2.0.0 addition — pairs with
// ensure_webhook_subscription / destroy_webhook_subscription to
// complete the universal Reconciler triple.
//
// Filter routing:
//   - {chave: X} → GET /v2/webhook/{chave} (single-subscription lookup)
//   - {}         → GET /v2/webhook (list all subscriptions on the
//                  account; BCB Pix API returns a webhooks[] array
//                  plus optional pagination metadata)
//
// Returns: {items: [...], cursor: ""} envelope on list, or the raw
// upstream payload on single-chave lookup.
//
// Idempotent — read-only.
func ObserveWebhookSubscriptions(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	if chave, _ := in["chave"].(string); chave != "" {
		var resp map[string]any
		if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/webhook/"+chave, nil, &resp); err != nil {
			return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
		}
		return resp, nil
	}
	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/webhook", nil, &resp); err != nil {
		return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
	}
	return resp, nil
}
