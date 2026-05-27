package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// UnregisterWebhookEndpoint DELETEs /v2/webhook/{chave}. EFI returns
// 204 on success and 404 when the webhook was already unregistered.
// We treat 404 as success — repeated calls are safe.
//
// Required input: chave.
func UnregisterWebhookEndpoint(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("unregister_webhook_endpoint: chave is required")
	}
	err := efiapi.DoRaw(ctx, c, http.MethodDelete, "/v2/webhook/"+chave, nil, nil)
	if err == nil {
		return map[string]any{"unregistered": true, "chave": chave}, nil
	}
	var apiErr *efiapi.EfiAPIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return map[string]any{"unregistered": true, "chave": chave, "already_absent": true}, nil
	}
	return nil, fmt.Errorf("unregister_webhook_endpoint: %w", err)
}
