package capabilities

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// GetStatement GETs /v2/cob with query parameters per BCB spec.
//
// Required input: inicio (RFC3339), fim (RFC3339).
// Optional:       status, page (int, default 0), page_size (int, default 100).
//
// Idempotent.
func GetStatement(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	inicio, _ := in["inicio"].(string)
	if inicio == "" {
		return nil, fmt.Errorf("get_statement: inicio is required (RFC3339)")
	}
	fim, _ := in["fim"].(string)
	if fim == "" {
		return nil, fmt.Errorf("get_statement: fim is required (RFC3339)")
	}

	q := url.Values{}
	q.Set("inicio", inicio)
	q.Set("fim", fim)
	if status, _ := in["status"].(string); status != "" {
		q.Set("status", status)
	}
	if page, ok := numericInt(in["page"]); ok {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize, ok := numericInt(in["page_size"]); ok {
		q.Set("page_size", strconv.Itoa(pageSize))
	}

	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/cob?"+q.Encode(), nil, &resp); err != nil {
		return nil, fmt.Errorf("get_statement: %w", err)
	}
	return resp, nil
}

// numericInt coerces an `any` payload value into an int. JSON
// unmarshalling produces float64 for numbers, so we coerce both float
// and int variants.
func numericInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		if x == "" {
			return 0, false
		}
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
