package adapter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/confidential"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
	"github.com/google/uuid"
)

type confidentialCallKey struct{}

// ConfidentialCall holds private material ONLY in the ExecuteHandler call's RAM.
// The SDK receives the original sealed Input, not this desired-state plaintext.
type ConfidentialCall struct {
	request  confidential.Request
	envelope contract.AdapterExecuteIntegrationRequest
	now      func() time.Time
	sealSeed []byte
}

// SDKIdempotency is a closed opaque per-intent token. Incoming free-form Metadata
// must not become event idempotency or reflect a provider value into that sink.
func (c *ConfidentialCall) SDKIdempotency() string {
	return "efi-confidential-v1:" + confidential.Fingerprint([]byte("efi.webhook.idempotency.v1\x00"+c.request.IntentSHA256))
}

func PrepareConfidential(ctx context.Context, envelope contract.AdapterExecuteIntegrationRequest, receiver *confidential.Receiver, now func() time.Time) (context.Context, *ConfidentialCall, error) {
	raw, err := json.Marshal(envelope.Input)
	if err != nil {
		return ctx, nil, confidential.ErrInvalid
	}
	var input confidential.Input
	if confidential.DecodeStrict(raw, &input) != nil {
		return ctx, nil, confidential.ErrInvalid
	}
	request, err := receiver.OpenInput(input.ConfidentialInput)
	if err != nil {
		return ctx, nil, err
	}
	if now == nil {
		now = time.Now
	}
	if validateConfidentialRequest(request, envelope) != nil || confidential.ValidateTime(request.Binding, now(), true) != nil {
		return ctx, nil, confidential.ErrInvalid
	}
	seed := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, seed); err != nil {
		return ctx, nil, confidential.ErrHeld
	}
	call := &ConfidentialCall{request: request, envelope: envelope, now: now, sealSeed: seed}
	ctx = efiapi.WithConfidentialDiagnostics(ctx)
	ctx = context.WithValue(ctx, confidentialCallKey{}, call)
	ctx = efiapi.WithMutationGuard(ctx, func() error { return confidential.ValidateTime(request.Binding, now(), false) })
	deadline, _ := time.Parse(time.RFC3339Nano, request.Binding.ExpiresAt)
	ctx = efiapi.WithMutationTimeout(ctx, func() time.Duration { return deadline.Sub(now()) })
	return ctx, call, nil
}

func validateConfidentialRequest(r confidential.Request, envelope contract.AdapterExecuteIntegrationRequest) error {
	b, p := r.Binding, r.Parameters
	u, err := url.Parse(p.CanonicalBase)
	start, startErr := time.Parse(time.RFC3339, p.Start)
	end, endErr := time.Parse(time.RFC3339, p.End)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(p.CanonicalBase, "#") || strings.IndexFunc(p.CanonicalBase, unicode.IsControl) >= 0 || p.CanonicalBase != strings.TrimSpace(p.CanonicalBase) || !capabilities.WebhookBaseURLValid(p.CanonicalBase) || startErr != nil || endErr != nil || !start.Before(end) || p.Page != 0 || p.PageSize < 1 || p.PageSize > 1000 || p.SkipMTLSValidation || !validPrivateKey(p.Key) {
		return confidential.ErrInvalid
	}
	if !canonicalID(b.InvocationID) || !canonicalID(b.AuditResourceID) || b.AuditResourceID == p.Key || b.InstanceName != envelope.Integration.Instance.Name || !confidential.SafeText(b.InstanceName, 128) || !hexValue(b.SourceRoot, 40) || !hexValue(b.ReleaseSHA256, 64) || !confidential.SafeText(b.Mode, 64) || !confidential.SafeText(b.WorkflowID, 128) || b.WorkflowVersion < 1 || !confidential.SafeText(b.StepID, 128) || b.Operation != envelope.Operation {
		return confidential.ErrInvalid
	}
	switch b.Purpose {
	case "list", "single", "confirm":
		if b.Operation != OperationObserveWebhookSubscriptions || p.BeforeState != "" {
			return confidential.ErrInvalid
		}
	case "ensure":
		if b.Operation != OperationEnsureWebhookSubscription || (p.BeforeState != "absent" && p.BeforeState != "canonical") {
			return confidential.ErrInvalid
		}
	default:
		return confidential.ErrInvalid
	}
	return nil
}
func validPrivateKey(value string) bool {
	if value == "" || len([]rune(value)) > 77 || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	if id, err := uuid.Parse(value); err == nil && id.String() != value {
		return false
	}
	return true
}
func canonicalID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id != uuid.Nil && id.String() == s
}
func hexValue(s string, n int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(s) == n && hex.EncodeToString(b) == s
}

func (c *ConfidentialCall) execute(ctx context.Context, operation string, input map[string]any) (map[string]any, error) {
	req := c.envelope
	req.Operation = operation
	req.Capability = operation
	req.Input = input
	response, err := ExecuteWithContext(ctx, req)
	if err != nil {
		if efiapi.MutationSucceeded(err) {
			return nil, efiapi.ErrConfidentialMutationSucceeded
		}
		return nil, confidential.ErrHeld
	}
	out, ok := response.Output.(map[string]any)
	if !ok {
		return nil, confidential.ErrHeld
	}
	if operation == OperationObserveWebhookSubscriptions {
		out, _ = capabilities.WebhookObservationOutput(out).(map[string]any)
	}
	return out, nil
}
func (c *ConfidentialCall) list(ctx context.Context) (map[string]any, confidential.Proof, error) {
	p := c.request.Parameters
	out, err := c.execute(ctx, OperationObserveWebhookSubscriptions, map[string]any{"inicio": p.Start, "fim": p.End, "page": p.Page, "page_size": p.PageSize})
	if err != nil {
		return nil, confidential.Proof{}, err
	}
	proof, err := c.listProof(out)
	return out, proof, err
}
func (c *ConfidentialCall) single(ctx context.Context) (map[string]any, confidential.Proof, error) {
	out, err := c.execute(ctx, OperationObserveWebhookSubscriptions, map[string]any{"chave": c.request.Parameters.Key})
	if err != nil {
		return nil, confidential.Proof{}, err
	}
	proof, err := c.singleProof(out)
	return out, proof, err
}

func ObserveConfidential(ctx context.Context, call *ConfidentialCall) (confidential.Output, error) {
	var out map[string]any
	var proof confidential.Proof
	var err error
	if call == nil {
		return confidential.Output{}, confidential.ErrInvalid
	}
	if call.request.Binding.Purpose == "list" {
		out, proof, err = call.list(ctx)
	} else {
		out, proof, err = call.single(ctx)
	}
	if err != nil {
		return confidential.Output{}, err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return confidential.Output{}, confidential.ErrHeld
	}
	return confidential.SealOutput(call.request, raw, nil, proof, bytes.NewReader(call.sealSeed))
}

func ensureConfidential(ctx context.Context, call *ConfidentialCall) (reconcilePayload, error) {
	guard, proof, err := call.list(ctx)
	if err != nil {
		return nil, err
	}
	k, _ := confidential.Decode32(call.request.ComparisonSecret)
	target, _ := confidential.Tag(k, confidential.KeyTagDomain, call.request.Binding.InvocationID, call.request.Parameters.Key)
	address, _ := confidential.Tag(k, confidential.URLTagDomain, call.request.Binding.InvocationID, call.request.Parameters.CanonicalBase)
	matched := false
	for _, row := range *proof.Rows {
		if row.KeyTag == target {
			if matched || row.URLTag != address {
				return nil, confidential.ErrHeld
			}
			matched = true
		}
	}
	guardRaw, err := json.Marshal(guard)
	if err != nil {
		return nil, confidential.ErrHeld
	}
	// Hold before any PUT if the full guard cannot fit the encrypted result.
	if len(guardRaw)+4096 > confidential.MaxOutputPlaintext {
		return nil, confidential.ErrHeld
	}
	// Both absence and an existing canonical registration require an actual
	// successful setter: EFI validates receiver mTLS during that PUT. A blind
	// canonical no-op cannot establish the original production evidence gate.
	if confidential.ValidateTime(call.request.Binding, call.now(), false) != nil {
		return nil, confidential.ErrExpired
	}
	legacy, err := call.execute(ctx, OperationEnsureWebhookSubscription, map[string]any{"chave": call.request.Parameters.Key, "webhook_url": call.request.Parameters.CanonicalBase, "skip_mtls_validation": false})
	if err != nil {
		if efiapi.MutationSucceeded(err) {
			return heldEnsure(call), nil
		}
		return nil, err
	}
	legacy["noop"] = false
	confirmed, _, err := call.single(ctx)
	if err != nil {
		return heldEnsure(call), nil
	}
	endpoint, ok := legacy["endpoint"].(string)
	if !ok || (endpoint != "v2" && endpoint != "v3") || legacy["ensured"] != true || legacy["chave"] != call.request.Parameters.Key {
		return heldEnsure(call), nil
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		return heldEnsure(call), nil
	}
	confirmationRaw, err := json.Marshal(confirmed)
	if err != nil {
		return heldEnsure(call), nil
	}
	yes, noop := true, false
	proof = confidential.Proof{SchemaVersion: 1, Kind: "ensure", Outcome: "confirmed", Ensured: &yes, Noop: &noop, KeyTag: target, Endpoint: endpoint}
	sealed, err := confidential.SealOutputWithConfirmation(call.request, raw, guardRaw, confirmationRaw, proof, bytes.NewReader(call.sealSeed))
	if err != nil {
		return heldEnsure(call), nil
	}
	result, err := outputMap(sealed)
	if err != nil {
		return heldEnsure(call), nil
	}
	return result, nil
}
func heldEnsure(call *ConfidentialCall) reconcilePayload {
	no := false
	out := confidential.Output{ID: call.request.Binding.AuditResourceID, Privacy: confidential.Privacy{SchemaVersion: 1, Algorithm: confidential.Algorithm, Ciphertext: ""}, Proof: confidential.Proof{SchemaVersion: 1, Kind: "ensure", Outcome: "held", Ensured: &no}}
	m, _ := outputMap(out)
	return m
}
func outputMap(out confidential.Output) (reconcilePayload, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, confidential.ErrHeld
	}
	var result reconcilePayload
	if json.Unmarshal(raw, &result) != nil {
		return nil, confidential.ErrHeld
	}
	return result, nil
}

func (c *ConfidentialCall) row(record map[string]any) (confidential.Row, error) {
	key, keyOK := record["chave"].(string)
	address, addressOK := record["webhookUrl"].(string)
	created, createdOK := record["criacao"].(string)
	t, err := time.Parse(time.RFC3339, created)
	start, _ := time.Parse(time.RFC3339, c.request.Parameters.Start)
	end, _ := time.Parse(time.RFC3339, c.request.Parameters.End)
	if !keyOK || !validPrivateKey(key) || !addressOK || !capabilities.WebhookObservedURLValid(address) || !createdOK || err != nil || t.Before(start) || t.After(end) {
		return confidential.Row{}, confidential.ErrHeld
	}
	k, err := confidential.Decode32(c.request.ComparisonSecret)
	if err != nil {
		return confidential.Row{}, err
	}
	keyTag, err := confidential.Tag(k, confidential.KeyTagDomain, c.request.Binding.InvocationID, key)
	if err != nil {
		return confidential.Row{}, err
	}
	urlTag, err := confidential.Tag(k, confidential.URLTagDomain, c.request.Binding.InvocationID, address)
	if err != nil {
		return confidential.Row{}, err
	}
	return confidential.Row{KeyTag: keyTag, URLTag: urlTag, CreationValid: true}, nil
}
func (c *ConfidentialCall) listProof(out map[string]any) (confidential.Proof, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return confidential.Proof{}, confidential.ErrHeld
	}
	var page struct {
		Provider   string                       `json:"provider"`
		Cursor     *string                      `json:"cursor"`
		Parameters *confidential.PageParameters `json:"parametros"`
		Provenance *confidential.PageProvenance `json:"page_provenance"`
		Webhooks   []map[string]any             `json:"webhooks"`
		Items      []map[string]any             `json:"items"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Provider != Provider || page.Cursor == nil || *page.Cursor != "" || page.Parameters == nil || page.Provenance == nil || page.Provenance.SchemaVersion != 1 || !page.Provenance.Validated || !page.Provenance.Complete || page.Parameters.Start != c.request.Parameters.Start || page.Parameters.End != c.request.Parameters.End || page.Parameters.Pagination.Page != 0 || page.Parameters.Pagination.PageSize != c.request.Parameters.PageSize || page.Parameters.Pagination.Pages != 1 || page.Parameters.Pagination.Total != len(page.Webhooks) || len(page.Webhooks) < 1 || len(page.Webhooks) != len(page.Items) || len(page.Webhooks) > c.request.Parameters.PageSize {
		return confidential.Proof{}, confidential.ErrHeld
	}
	rows := make([]confidential.Row, 0, len(page.Webhooks))
	seen := map[string]bool{}
	for i, record := range page.Webhooks {
		// Public audit identity must never be an account's actual Pix key,
		// including another EVP returned by this complete own-LIST.
		if record["chave"] == c.request.Binding.AuditResourceID {
			return confidential.Proof{}, confidential.ErrHeld
		}
		a, e := confidential.CanonicalJSON(record)
		b, f := confidential.CanonicalJSON(page.Items[i])
		if e != nil || f != nil || !bytes.Equal(a, b) {
			return confidential.Proof{}, confidential.ErrHeld
		}
		row, e := c.row(record)
		if e != nil || seen[row.KeyTag] {
			return confidential.Proof{}, confidential.ErrHeld
		}
		seen[row.KeyTag] = true
		rows = append(rows, row)
	}
	return confidential.Proof{SchemaVersion: 1, Kind: "list", Outcome: "confirmed", Provider: Provider, Cursor: page.Cursor, Parameters: page.Parameters, PageProvenance: page.Provenance, Rows: &rows}, nil
}
func (c *ConfidentialCall) singleProof(out map[string]any) (confidential.Proof, error) {
	row, err := c.row(out)
	if err != nil || out["provider"] != Provider || out["chave"] != c.request.Parameters.Key || out["webhookUrl"] != c.request.Parameters.CanonicalBase {
		return confidential.Proof{}, confidential.ErrHeld
	}
	items, ok := out["items"].([]any)
	if !ok || len(items) != 1 {
		return confidential.Proof{}, confidential.ErrHeld
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		return confidential.Proof{}, confidential.ErrHeld
	}
	copyOut := map[string]any{}
	for key, value := range out {
		if key != "items" && key != "cursor" {
			copyOut[key] = value
		}
	}
	a, e := confidential.CanonicalJSON(copyOut)
	b, f := confidential.CanonicalJSON(item)
	cursor, ok := out["cursor"].(string)
	if e != nil || f != nil || !bytes.Equal(a, b) || !ok || cursor != "" {
		return confidential.Proof{}, confidential.ErrHeld
	}
	yes := true
	return confidential.Proof{SchemaVersion: 1, Kind: "single", Outcome: "confirmed", Provider: Provider, Cursor: &cursor, KeyTag: row.KeyTag, URLTag: row.URLTag, CreationValid: &yes}, nil
}
