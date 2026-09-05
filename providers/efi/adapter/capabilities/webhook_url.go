package capabilities

import (
	"fmt"
	"net/url"
	"strings"
)

const efiWebhookDeliverySuffix = "/pix"

// normalizeWebhookBaseURL validates the URL sent to EFI when registering a
// Pix webhook. EFI probes this exact URL during registration and appends
// "/pix" when delivering real notifications, so callers must provide the
// receiver base path rather than the final callback path.
func normalizeWebhookBaseURL(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("webhook_url is required")
	}

	parsed, err := url.Parse(normalized)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("webhook_url must be an absolute HTTPS URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("webhook_url must use HTTPS")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("webhook_url must not contain userinfo")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("webhook_url must not contain a fragment")
	}

	path := strings.ToLower(strings.TrimRight(parsed.Path, "/"))
	if strings.HasSuffix(path, efiWebhookDeliverySuffix) {
		return "", fmt.Errorf("webhook_url must be the receiver base URL without /pix because EFI appends /pix on delivery")
	}
	return normalized, nil
}

// sanitizeObservedWebhookURLs removes URL-embedded secret values from normal
// observe responses. EFI documents query-parameter HMACs for skip-mTLS
// deployments; returning those values through workflow history, logs, or a
// surface would violate the integration secret boundary.
func sanitizeObservedWebhookURLs(resp map[string]any) {
	if resp == nil {
		return
	}
	sanitizeWebhookURLField(resp, "webhookUrl")
	sanitizeWebhookURLField(resp, "url")

	switch rows := resp["webhooks"].(type) {
	case []any:
		for _, row := range rows {
			if item, ok := row.(map[string]any); ok {
				sanitizeWebhookURLField(item, "webhookUrl")
				sanitizeWebhookURLField(item, "url")
			}
		}
	case []map[string]any:
		for _, item := range rows {
			sanitizeWebhookURLField(item, "webhookUrl")
			sanitizeWebhookURLField(item, "url")
		}
	}
}

func sanitizeWebhookURLField(item map[string]any, key string) {
	raw, ok := item[key].(string)
	if !ok || raw == "" {
		return
	}
	item[key] = redactURLSecrets(raw)
}

func redactURLSecrets(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "[redacted invalid webhook URL]"
	}
	parsed.User = nil
	parsed.Fragment = ""
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key, values := range query {
			for i, value := range values {
				if value != "" {
					values[i] = "[redacted]"
				}
			}
			query[key] = values
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}
