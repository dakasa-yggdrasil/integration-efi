package capabilities

// WebhookObservationOutput is the legacy actual-handler DTO transformation.
// Sharing it below message preserves complete raw/additive fields for sealing
// without creating a message/adapter import cycle or a second imagined DTO.
func WebhookObservationOutput(value any) any {
	output, ok := value.(map[string]any)
	if !ok {
		return value
	}
	var items []any
	if rows, list := output["webhooks"]; list {
		if rows, ok := rows.([]any); ok {
			for _, row := range rows {
				if _, ok := row.(map[string]any); ok {
					items = append(items, row)
				}
			}
		}
	} else {
		_, key := output["chave"].(string)
		_, address := output["webhookUrl"].(string)
		if key || address {
			item := make(map[string]any, len(output))
			for k, v := range output {
				item[k] = v
			}
			items = []any{item}
		}
	}
	output["items"] = items
	if _, exists := output["cursor"]; !exists {
		output["cursor"] = ""
	}
	return output
}

// WebhookObservedURLValid does not canonicalize the string used for HMAC tags.
func WebhookObservedURLValid(raw string) bool { return webhookObservedURLValid(raw) }

// WebhookBaseURLValid retains the setter's base-path contract for confidential
// no-op adoption as well as for an actual PUT.
func WebhookBaseURLValid(raw string) bool {
	normalized, err := normalizeWebhookBaseURL(raw)
	return err == nil && normalized == raw
}
