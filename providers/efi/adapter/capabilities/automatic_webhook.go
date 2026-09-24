package capabilities

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// Pix Automatico webhook registrations.
//
// EFI keeps one webhook registration per API application for each of two
// Pix Automatico endpoints, and neither is keyed by a Pix key:
//
//   - kind "rec":  PUT/GET /v2/webhookrec, recurrence lifecycle notifications.
//     EFI probes the registered URL and delivers to <registered URL>/rec.
//   - kind "cobr": PUT/GET /v2/webhookcobr, recurring charge notifications.
//     EFI probes the registered URL and delivers to <registered URL>/cobr.
//
// These capabilities never call /v2/webhook/{chave} (the Pix key webhook
// belongs to ensure_webhook_subscription) and never send the
// x-skip-mtls-checking header: the receiver must authenticate EFI with mTLS.
// There is intentionally no destroy capability for this resource.
const (
	AutomaticWebhookKindRecurrence = "rec"
	AutomaticWebhookKindCharge     = "cobr"
)

var automaticWebhookEndpoints = map[string]string{
	AutomaticWebhookKindRecurrence: "/v2/webhookrec",
	AutomaticWebhookKindCharge:     "/v2/webhookcobr",
}

// AutomaticWebhookResourceID returns the stable provider-side identity of
// one automatic webhook registration, which is the EFI endpoint name
// ("webhookrec" or "webhookcobr"). It returns "" for an unknown kind.
func AutomaticWebhookResourceID(kind string) string {
	path, ok := automaticWebhookEndpoints[kind]
	if !ok {
		return ""
	}
	return strings.TrimPrefix(path, "/v2/")
}

// automaticWebhookForbiddenInputs are refused before any provider call.
// skip_mtls_validation would ask EFI to skip the mTLS handshake on
// delivery; chave would suggest the Pix key webhook path, which these
// capabilities never touch.
var automaticWebhookForbiddenInputs = []struct {
	key    string
	reason string
}{
	{"skip_mtls_validation", "skip_mtls_validation is not accepted: automatic webhook deliveries must be authenticated by mTLS"},
	{"chave", "chave is not accepted: automatic webhooks are registered per API application, not per Pix key"},
}

type automaticWebhookState struct {
	registered bool
	url        string
	createdAt  string
}

// EnsureAutomaticWebhook registers the Pix Automatico webhook of one kind.
//
// Required input: kind ("rec" or "cobr"), webhook_url.
//
// It reads the current registration first and issues exactly one PUT only
// when the registration is absent or points elsewhere. It then reads the
// registration back and fails unless the provider returns exactly
// webhook_url. It never retries the PUT. Idempotent: a matching
// registration is adopted without a mutation.
func EnsureAutomaticWebhook(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	const op = "ensure_automatic_webhook"
	if err := refuseAutomaticWebhookInputs(in); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	kind, path, err := automaticWebhookKind(in)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	desired, err := normalizeAutomaticWebhookURL(stringValue(in, "webhook_url"), "webhook_url")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	current, err := readAutomaticWebhook(ctx, c, path)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s registration before mutation: %w", op, kind, err)
	}

	changed := false
	if !current.registered || current.url != desired {
		if err := efiapi.DoRaw(ctx, c, http.MethodPut, path, map[string]any{"webhookUrl": desired}, nil); err != nil {
			return nil, fmt.Errorf("%s: register %s webhook: %w", op, kind, err)
		}
		changed = true
	}

	readback, err := readAutomaticWebhook(ctx, c, path)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s registration back (the registration may have been applied; observe before any new attempt): %w", op, kind, err)
	}
	if !readback.registered || readback.url != desired {
		return nil, fmt.Errorf("%s: %s readback does not equal webhook_url (registered=%t)", op, kind, readback.registered)
	}

	out := automaticWebhookOutput(kind, path, readback)
	out["ensured"] = true
	out["changed"] = changed
	return out, nil
}

// ObserveAutomaticWebhooks reads the Pix Automatico webhook of one kind.
//
// Required input: kind ("rec" or "cobr").
// Optional input: expected_webhook_url. When present, the call fails unless
// the provider returns exactly that URL, which makes the observation a
// readback gate for a workflow.
//
// An absent registration (HTTP 404, or a response without webhookUrl) is
// reported as registered=false, never as an error, unless
// expected_webhook_url was given. Read-only.
func ObserveAutomaticWebhooks(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	const op = "observe_automatic_webhooks"
	if err := refuseAutomaticWebhookInputs(in); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	kind, path, err := automaticWebhookKind(in)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	expected := ""
	expectedValue, hasExpected := in["expected_webhook_url"]
	if hasExpected {
		raw, ok := expectedValue.(string)
		if !ok {
			return nil, fmt.Errorf("%s: expected_webhook_url must be a string", op)
		}
		expected, err = normalizeAutomaticWebhookURL(raw, "expected_webhook_url")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	state, err := readAutomaticWebhook(ctx, c, path)
	if err != nil {
		return nil, fmt.Errorf("%s: read %s registration: %w", op, kind, err)
	}
	out := automaticWebhookOutput(kind, path, state)
	if hasExpected {
		if !state.registered || state.url != expected {
			return nil, fmt.Errorf("%s: %s readback does not equal expected_webhook_url (registered=%t)", op, kind, state.registered)
		}
		out["matches_expected"] = true
	}
	return out, nil
}

func refuseAutomaticWebhookInputs(in map[string]any) error {
	for _, forbidden := range automaticWebhookForbiddenInputs {
		if _, present := in[forbidden.key]; present {
			return errors.New(forbidden.reason)
		}
	}
	return nil
}

func automaticWebhookKind(in map[string]any) (string, string, error) {
	raw, ok := in["kind"].(string)
	if !ok {
		return "", "", errors.New(`kind is required and must be "rec" or "cobr"`)
	}
	kind := strings.TrimSpace(raw)
	path, ok := automaticWebhookEndpoints[kind]
	if !ok {
		return "", "", errors.New(`kind must be "rec" or "cobr"`)
	}
	return kind, path, nil
}

// readAutomaticWebhook issues one GET. A 404 or a 2xx body without
// webhookUrl means nothing is registered. Any other provider error fails.
func readAutomaticWebhook(ctx context.Context, c *efiapi.EfiClient, path string) (automaticWebhookState, error) {
	var resp map[string]any
	err := efiapi.DoRaw(ctx, c, http.MethodGet, path, nil, &resp)
	if err != nil {
		var apiErr *efiapi.EfiAPIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return automaticWebhookState{}, nil
		}
		return automaticWebhookState{}, err
	}
	registered, _ := resp["webhookUrl"].(string)
	if strings.TrimSpace(registered) == "" {
		return automaticWebhookState{}, nil
	}
	createdAt, _ := resp["criacao"].(string)
	return automaticWebhookState{registered: true, url: registered, createdAt: createdAt}, nil
}

func automaticWebhookOutput(kind, path string, state automaticWebhookState) map[string]any {
	out := map[string]any{
		"kind":        kind,
		"endpoint":    path,
		"resource_id": AutomaticWebhookResourceID(kind),
		"registered":  state.registered,
	}
	if state.registered {
		// The comparison above uses the raw provider value. Only the
		// returned copy is redacted, so a query secret registered by
		// another path never enters workflow history.
		out["webhook_url"] = redactURLSecrets(state.url)
		if state.createdAt != "" {
			out["created_at"] = state.createdAt
		}
	}
	return out
}

// normalizeAutomaticWebhookURL validates a receiver base URL for a Pix
// Automatico webhook without echoing the value in errors. The URL must be
// an absolute https URL with a host, and no userinfo, query, fragment,
// escaped path or trailing slash. A query is refused because EFI's
// skip-mTLS mode authenticates deliveries with an HMAC in the query
// string, and because query values would enter workflow history. The last
// path segment cannot be rec, cobr or pix because EFI appends the delivery
// suffix itself.
func normalizeAutomaticWebhookURL(raw, field string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if !strings.HasPrefix(normalized, "https://") {
		return "", fmt.Errorf("%s must be an absolute URL starting with https://", field)
	}
	parsed, err := url.Parse(normalized)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("%s must be an absolute HTTPS URL with a host", field)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%s must not contain userinfo", field)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", fmt.Errorf("%s must not contain a query: deliveries are authenticated by mTLS, not by a value in the URL", field)
	}
	if parsed.Fragment != "" || strings.Contains(normalized, "#") {
		return "", fmt.Errorf("%s must not contain a fragment", field)
	}
	if parsed.RawPath != "" {
		return "", fmt.Errorf("%s must not contain an escaped path", field)
	}
	if strings.HasSuffix(parsed.Path, "/") {
		return "", fmt.Errorf("%s must not end with a slash because EFI appends the delivery suffix", field)
	}
	segments := strings.Split(parsed.Path, "/")
	switch strings.ToLower(segments[len(segments)-1]) {
	case "rec", "cobr", "pix":
		return "", fmt.Errorf("%s must be the receiver base URL without /rec, /cobr or /pix because EFI appends the delivery suffix", field)
	}
	return normalized, nil
}
