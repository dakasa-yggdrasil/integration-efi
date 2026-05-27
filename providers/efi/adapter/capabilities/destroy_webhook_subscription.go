package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// DestroyWebhookSubscription DELETEs /v2/webhook/{chave}. EFI returns
// 204 on success and 404 when the subscription was already removed.
// We treat 404 as success — repeated calls are safe (the caller's
// desired state was "this subscription should not exist," and it
// does not). v2.0.0 rename of unregister_webhook_endpoint.
//
// Required input: chave.
func DestroyWebhookSubscription(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("destroy_webhook_subscription: chave is required")
	}
	err := efiapi.DoRaw(ctx, c, http.MethodDelete, "/v2/webhook/"+chave, nil, nil)
	if err == nil {
		return map[string]any{"destroyed": true, "chave": chave}, nil
	}
	var apiErr *efiapi.EfiAPIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return map[string]any{"destroyed": true, "chave": chave, "already_absent": true}, nil
	}
	return nil, fmt.Errorf("destroy_webhook_subscription: %w", err)
}
