package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// RegisterWebhookEndpoint PUTs /v2/webhook/{chave} (with v3 fallback
// on 404) per EFI's webhook surface. Some EFI accounts only accept
// the v3 path — the fallback mirrors the legacy
// `client/efi-bank.go:383-391` behavior in dakasa-identities.
//
// Required input: chave, webhook_url.
// Optional:       skip_mtls_validation (bool) — adds
//                 `x-skip-mtls-checking: true` request header.
//
// Idempotent — repeated calls overwrite the registration.
func RegisterWebhookEndpoint(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("register_webhook_endpoint: chave is required")
	}
	webhookURL, _ := in["webhook_url"].(string)
	if webhookURL == "" {
		return nil, fmt.Errorf("register_webhook_endpoint: webhook_url is required")
	}

	body := map[string]any{"webhookUrl": webhookURL}
	headers := map[string]string{}
	if skip, _ := in["skip_mtls_validation"].(bool); skip {
		headers["x-skip-mtls-checking"] = "true"
	}

	err := efiapi.DoRawWithHeaders(ctx, c, http.MethodPut, "/v2/webhook/"+chave, body, nil, headers)
	if err == nil {
		return map[string]any{"registered": true, "chave": chave, "endpoint": "v2"}, nil
	}

	var apiErr *efiapi.EfiAPIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		// Some EFI accounts only accept the v3 path.
		if err2 := efiapi.DoRawWithHeaders(ctx, c, http.MethodPut, "/v3/gn/webhook/"+chave, body, nil, headers); err2 == nil {
			return map[string]any{"registered": true, "chave": chave, "endpoint": "v3"}, nil
		} else {
			return nil, fmt.Errorf("register_webhook_endpoint: v2 returned 404, v3 fallback also failed: %w", err2)
		}
	}
	return nil, fmt.Errorf("register_webhook_endpoint: %w", err)
}
