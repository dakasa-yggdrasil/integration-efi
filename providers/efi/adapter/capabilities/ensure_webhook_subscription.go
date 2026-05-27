package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// EnsureWebhookSubscription PUTs /v2/webhook/{chave} (with v3 fallback
// on 404) per EFI's webhook surface. Some EFI accounts only accept the
// v3 path — the fallback mirrors the legacy
// `client/efi-bank.go:383-391` behavior in dakasa-identities. v2.0.0
// rename of register_webhook_endpoint: the canonical ensure_ prefix
// reflects that repeat PUTs reconcile (overwrite URL/headers) and
// adopt any pre-existing subscription with the same chave.
//
// Required input: chave, webhook_url.
// Optional:       skip_mtls_validation (bool) — adds
//                 `x-skip-mtls-checking: true` request header.
//
// Idempotent — repeated calls reconcile the same subscription.
func EnsureWebhookSubscription(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("ensure_webhook_subscription: chave is required")
	}
	webhookURL, _ := in["webhook_url"].(string)
	if webhookURL == "" {
		return nil, fmt.Errorf("ensure_webhook_subscription: webhook_url is required")
	}

	body := map[string]any{"webhookUrl": webhookURL}
	headers := map[string]string{}
	if skip, _ := in["skip_mtls_validation"].(bool); skip {
		headers["x-skip-mtls-checking"] = "true"
	}

	err := efiapi.DoRawWithHeaders(ctx, c, http.MethodPut, "/v2/webhook/"+chave, body, nil, headers)
	if err == nil {
		return map[string]any{"ensured": true, "chave": chave, "endpoint": "v2"}, nil
	}

	var apiErr *efiapi.EfiAPIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		// Some EFI accounts only accept the v3 path.
		if err2 := efiapi.DoRawWithHeaders(ctx, c, http.MethodPut, "/v3/gn/webhook/"+chave, body, nil, headers); err2 == nil {
			return map[string]any{"ensured": true, "chave": chave, "endpoint": "v3"}, nil
		} else {
			return nil, fmt.Errorf("ensure_webhook_subscription: v2 returned 404, v3 fallback also failed: %w", err2)
		}
	}
	return nil, fmt.Errorf("ensure_webhook_subscription: %w", err)
}
