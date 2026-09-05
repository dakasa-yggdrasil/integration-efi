package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// ObserveWebhookSubscriptions implements the canonical observe_ side
// of the webhook_subscription resource. v2.0.0 addition — pairs with
// ensure_webhook_subscription / destroy_webhook_subscription to
// complete the universal Reconciler triple.
//
// Filter routing:
//   - {chave: X} → GET /v2/webhook/{chave} (single-subscription lookup)
//   - {inicio, fim[, page, page_size, cursor]} → GET /v2/webhook
//     (windowed list; EFI requires inicio/fim and accepts the BCB Pix
//     paginacao.paginaAtual / paginacao.itensPorPagina query fields)
//
// Returns the normalized upstream payload. URL query values are redacted so a
// registered HMAC cannot enter workflow history. The list variant also includes
// a top-level cursor containing the next page number when another page exists.
//
// Idempotent — read-only.
func ObserveWebhookSubscriptions(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	chaveValue, hasChave := in["chave"]
	if hasChave {
		chave, ok := chaveValue.(string)
		if !ok || strings.TrimSpace(chave) == "" {
			return nil, fmt.Errorf("observe_webhook_subscriptions: chave must be a non-empty string")
		}
		if hasAnyInput(in, "inicio", "fim", "page", "page_size", "cursor") {
			return nil, fmt.Errorf("observe_webhook_subscriptions: chave and time-range filters are mutually exclusive")
		}
		chave = strings.TrimSpace(chave)
		var resp map[string]any
		if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/webhook/"+url.PathEscape(chave), nil, &resp); err != nil {
			return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
		}
		sanitizeObservedWebhookURLs(resp)
		return resp, nil
	}

	inicio, fim, err := webhookTimeRange(in)
	if err != nil {
		return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
	}

	q := url.Values{}
	q.Set("inicio", inicio)
	q.Set("fim", fim)

	page, hasPage, err := paginationInput(in, "page", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
	}
	cursor := ""
	if value, present := in["cursor"]; present {
		var ok bool
		cursor, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("observe_webhook_subscriptions: cursor: must be an integer string")
		}
		cursor = strings.TrimSpace(cursor)
	}
	if hasPage && cursor != "" {
		return nil, fmt.Errorf("observe_webhook_subscriptions: page and cursor are mutually exclusive")
	}
	if cursor != "" {
		page, err = parsePaginationInteger(cursor, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("observe_webhook_subscriptions: cursor: %w", err)
		}
		hasPage = true
	}
	if hasPage {
		q.Set("paginacao.paginaAtual", strconv.Itoa(page))
	}

	pageSize, hasPageSize, err := paginationInput(in, "page_size", 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
	}
	if hasPageSize {
		q.Set("paginacao.itensPorPagina", strconv.Itoa(pageSize))
	}

	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodGet, "/v2/webhook?"+q.Encode(), nil, &resp); err != nil {
		return nil, fmt.Errorf("observe_webhook_subscriptions: %w", err)
	}
	if resp == nil {
		resp = map[string]any{}
	}
	sanitizeObservedWebhookURLs(resp)
	resp["cursor"] = nextWebhookPageCursor(resp)
	return resp, nil
}

func webhookTimeRange(in map[string]any) (string, string, error) {
	inicio, inicioOK := in["inicio"].(string)
	fim, fimOK := in["fim"].(string)
	inicio = strings.TrimSpace(inicio)
	fim = strings.TrimSpace(fim)
	if !inicioOK || !fimOK || inicio == "" || fim == "" {
		return "", "", fmt.Errorf("requires either {chave} or {inicio, fim}")
	}
	start, err := time.Parse(time.RFC3339, inicio)
	if err != nil {
		return "", "", fmt.Errorf("inicio must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, fim)
	if err != nil {
		return "", "", fmt.Errorf("fim must be RFC3339")
	}
	if end.Before(start) {
		return "", "", fmt.Errorf("fim must be greater than or equal to inicio")
	}
	return inicio, fim, nil
}

func hasAnyInput(in map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, present := in[key]; present {
			return true
		}
	}
	return false
}

func paginationInput(in map[string]any, key string, min, max int) (int, bool, error) {
	value, present := in[key]
	if !present {
		return 0, false, nil
	}
	n, err := parsePaginationInteger(value, min, max)
	if err != nil {
		return 0, false, fmt.Errorf("%s: %w", key, err)
	}
	return n, true, nil
}

func parsePaginationInteger(value any, min, max int) (int, error) {
	var n int
	switch v := value.(type) {
	case int:
		n = v
	case int64:
		if v > int64(^uint(0)>>1) || v < -int64(^uint(0)>>1)-1 {
			return 0, fmt.Errorf("must be an integer")
		}
		n = int(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v > float64(^uint(0)>>1) || v < -float64(^uint(0)>>1)-1 {
			return 0, fmt.Errorf("must be an integer")
		}
		n = int(v)
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil || int64(int(parsed)) != parsed {
			return 0, fmt.Errorf("must be an integer")
		}
		n = int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("must be an integer")
		}
		n = parsed
	default:
		return 0, fmt.Errorf("must be an integer")
	}
	if n < min {
		return 0, fmt.Errorf("must be at least %d", min)
	}
	if max > 0 && n > max {
		return 0, fmt.Errorf("must be at most %d", max)
	}
	return n, nil
}

func nextWebhookPageCursor(resp map[string]any) string {
	params, _ := resp["parametros"].(map[string]any)
	pagination, _ := params["paginacao"].(map[string]any)
	current, err := parsePaginationInteger(pagination["paginaAtual"], 0, 0)
	if err != nil {
		return ""
	}
	total, err := parsePaginationInteger(pagination["quantidadeDePaginas"], 0, 0)
	if err != nil || current+1 >= total {
		return ""
	}
	return strconv.Itoa(current + 1)
}
