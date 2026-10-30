package message

// webhookObservationOutput preserves the public raw provider observation while
// retaining the items/cursor envelope existing SDK consumers already receive.
// The items alias cannot establish page completeness; callers must validate the
// original provider parameters and every original record.
func webhookObservationOutput(value any) any {
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
			// Copy before adding items, avoiding a self-referential output.
			item := make(map[string]any, len(output))
			for name, field := range output {
				item[name] = field
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
