package adapter

import (
	"context"
	"encoding/json"
	"os"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/events"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

// reconcilePayload is the typed envelope reconciler impls forward into
// the existing Execute() switch — the production runtime path the EFI
// adapter has shipped since v1.0.0. The reconcile.RegisterReconciler
// wiring is the Go-level expression of the universal convention; both
// paths reach the same per-capability handlers. The Option B hybrid
// bridge mirrors the integration-slack / integration-stripe pattern.
type reconcilePayload map[string]any

// dispatchFn is the function the per-resource reconcilers invoke. They
// reuse the existing Execute() path so legacy alias resolution and
// auth-loading semantics stay identical between the SDK-driven path
// and the AMQP/HTTP transport path.
type dispatchFn func(op string, instanceID string, in reconcilePayload) (reconcilePayload, error)

func defaultDispatch(op string, instanceID string, in reconcilePayload) (reconcilePayload, error) {
	integ := integrationFromPayload(in, instanceID)
	// Strip the bridge metadata before forwarding to Execute so the
	// legacy switch sees a clean input map.
	cleanInput := make(map[string]any, len(in))
	for k, v := range in {
		if k == "_integration" || k == "instance_id" {
			continue
		}
		cleanInput[k] = v
	}
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   op,
		Input:       cleanInput,
		Integration: integ,
	})
	if err != nil {
		return nil, err
	}
	if m, ok := resp.Output.(map[string]any); ok {
		return reconcilePayload(m), nil
	}
	return reconcilePayload{"output": resp.Output}, nil
}

// integrationFromPayload reconstructs the per-request integration
// context from input metadata lifted onto the payload by the
// ExecuteHandler bridge in providers/efi/message/execute.go. The bridge
// stuffs the full Integration shape under "_integration" + the
// instance_id under "instance_id"; this helper unmarshals it back.
// Falls back to a minimal context (only Instance.Name) when only the
// top-level instance_id is available — single-instance flows still work.
func integrationFromPayload(in reconcilePayload, fallback string) contract.AdapterExecuteIntegrationContext {
	if in == nil {
		if fallback == "" {
			return contract.AdapterExecuteIntegrationContext{}
		}
		return contract.AdapterExecuteIntegrationContext{
			Instance: contract.ManifestReference{Name: fallback},
		}
	}
	if raw, ok := in["_integration"].(map[string]any); ok {
		// Best-effort: marshal then unmarshal back into the typed
		// struct so nested fields (InstanceSpec.Credentials,
		// InstanceSpec.Config) survive.
		b, err := json.Marshal(raw)
		if err == nil {
			var integ contract.AdapterExecuteIntegrationContext
			if jerr := json.Unmarshal(b, &integ); jerr == nil {
				return integ
			}
		}
	}
	if v, ok := in["instance_id"].(string); ok && v != "" {
		return contract.AdapterExecuteIntegrationContext{
			Instance: contract.ManifestReference{Name: v},
		}
	}
	if fallback == "" {
		return contract.AdapterExecuteIntegrationContext{}
	}
	return contract.AdapterExecuteIntegrationContext{
		Instance: contract.ManifestReference{Name: fallback},
	}
}

// chargeReconciler wires ensure_charge / observe_charges / destroy_charge
// through the legacy Execute() switch. EnsureCharge supports both auto-
// generated txid (POST /v2/cob) and caller-supplied txid (PUT /v2/cob/{txid}).
type chargeReconciler struct {
	instanceID string
	dispatch   dispatchFn
}

func newChargeReconciler(instanceID string) *chargeReconciler {
	return &chargeReconciler{instanceID: instanceID, dispatch: defaultDispatch}
}

func (r *chargeReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureCharge, r.instanceID, d)
}

func (r *chargeReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObserveCharges, r.instanceID, reconcilePayload(filter))
	if err != nil {
		return nil, "", err
	}
	// EFI's observe_charges returns the upstream BCB Pix payload verbatim.
	// For single-txid lookup the response IS the charge; for range queries
	// the response carries a "cobs" array (BCB convention) plus optional
	// pagination metadata. Normalize both shapes into the SDK's items/cursor
	// envelope so reconcile dispatch can hand back a uniform shape.
	return extractObserveCharges(out), stringValue(out, "cursor"), nil
}

func (r *chargeReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyCharge, r.instanceID, reconcilePayload{"txid": ref})
	return err
}

// dueChargeReconciler wires ensure_due_charge / observe_charges /
// destroy_charge against the EFI cobv (due charge) variant. Note that
// observe and destroy reuse the charge endpoints — BCB Pix exposes the
// same /v2/cob/{txid} read path for both cob and cobv records (the API
// returns the appropriate calendar shape based on which endpoint
// created the txid). Destroy on a cobv reuses the immediate-charge
// destroy contract (same PATCH path).
type dueChargeReconciler struct {
	instanceID string
	dispatch   dispatchFn
}

func newDueChargeReconciler(instanceID string) *dueChargeReconciler {
	return &dueChargeReconciler{instanceID: instanceID, dispatch: defaultDispatch}
}

func (r *dueChargeReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureDueCharge, r.instanceID, d)
}

func (r *dueChargeReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	// Reuse observe_charges — BCB Pix returns the same shape for cobv
	// records when queried by txid through /v2/cob/{txid}.
	out, err := r.dispatch(OperationObserveCharges, r.instanceID, reconcilePayload(filter))
	if err != nil {
		return nil, "", err
	}
	return extractObserveCharges(out), stringValue(out, "cursor"), nil
}

func (r *dueChargeReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyCharge, r.instanceID, reconcilePayload{"txid": ref})
	return err
}

// webhookSubscriptionReconciler wires the three webhook subscription
// capabilities. Identity is the Pix key (chave) — the subscription is
// keyed by chave on the EFI side, so destroy.ref carries the chave.
type webhookSubscriptionReconciler struct {
	instanceID string
	dispatch   dispatchFn
}

func newWebhookSubscriptionReconciler(instanceID string) *webhookSubscriptionReconciler {
	return &webhookSubscriptionReconciler{instanceID: instanceID, dispatch: defaultDispatch}
}

func (r *webhookSubscriptionReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureWebhookSubscription, r.instanceID, d)
}

func (r *webhookSubscriptionReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObserveWebhookSubscriptions, r.instanceID, reconcilePayload(filter))
	if err != nil {
		return nil, "", err
	}
	return extractWebhookSubscriptions(out), stringValue(out, "cursor"), nil
}

func (r *webhookSubscriptionReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyWebhookSubscription, r.instanceID, reconcilePayload{"chave": ref})
	return err
}

// extractObserveCharges normalizes the BCB Pix /v2/cob response into
// the SDK's items[] envelope. Two upstream shapes:
//   - Range query: {cobs: [...], parametros: {...}} — extract cobs.
//   - Single-txid: a single charge object (no array). Wrap it as one item.
func extractObserveCharges(resp reconcilePayload) []reconcilePayload {
	if resp == nil {
		return nil
	}
	if items := extractItemsByKey(resp, "cobs"); len(items) > 0 {
		return items
	}
	if items := extractItemsByKey(resp, "items"); len(items) > 0 {
		return items
	}
	// Single-charge response — the response IS the charge. Only wrap
	// it as an item when it carries a txid (the BCB Pix identity).
	if _, hasTxid := resp["txid"].(string); hasTxid {
		return []reconcilePayload{resp}
	}
	return nil
}

// extractWebhookSubscriptions normalizes the BCB Pix /v2/webhook
// response into items[]. Two upstream shapes:
//   - List query: {webhooks: [...], parametros: {...}}
//   - Single-chave: a single subscription object (no array).
func extractWebhookSubscriptions(resp reconcilePayload) []reconcilePayload {
	if resp == nil {
		return nil
	}
	if items := extractItemsByKey(resp, "webhooks"); len(items) > 0 {
		return items
	}
	if items := extractItemsByKey(resp, "items"); len(items) > 0 {
		return items
	}
	// Single-subscription response — wrap when it carries chave/webhookUrl.
	if _, hasChave := resp["chave"].(string); hasChave {
		return []reconcilePayload{resp}
	}
	if _, hasURL := resp["webhookUrl"].(string); hasURL {
		return []reconcilePayload{resp}
	}
	return nil
}

// extractItemsByKey pulls a list from resp[key], converting each
// map[string]any element into a reconcilePayload.
func extractItemsByKey(resp reconcilePayload, key string) []reconcilePayload {
	if resp == nil {
		return nil
	}
	switch raw := resp[key].(type) {
	case []map[string]any:
		out := make([]reconcilePayload, 0, len(raw))
		for _, item := range raw {
			out = append(out, reconcilePayload(item))
		}
		return out
	case []any:
		out := make([]reconcilePayload, 0, len(raw))
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				out = append(out, reconcilePayload(m))
			}
		}
		return out
	}
	return nil
}

func stringValue(m reconcilePayload, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// WireReconcilers installs reconcile.RegisterReconciler handlers for
// each EFI managed resource (charge, due_charge, webhook_subscription).
// The pre-v2.0.0 legacy capability names are kept alive through
// reconcile.WithLegacyNames so callers that still send e.g.
// "create_charge" route to ensure_charge with a WARN log entry. The
// shim removal target moved to SDK v0.7.0; this adapter drops the
// legacy lists at v3.0.0.
//
// Production wiring (v0.7.0+): main() calls WireReconcilers BEFORE
// registering describe/execute, and the controllers/message
// ExecuteHandler routes inbound traffic through reconcile.Dispatch —
// activating §6.5 mutation event auto-emission for every operator
// request. instanceID is the FALLBACK passed when the inbound
// envelope carries no instance_id; the reconciler dispatch helpers
// prefer the payload-bound value (integrationFromPayload).
//
// SDK v0.6.0+: a MutationEvent emitter is wired through WithEmitter +
// WithProvider so successful Ensure/Destroy calls auto-emit §6.5
// mutation events to yggdrasil-core. When YGGDRASIL_CORE_URL is empty
// the emitter degrades to NoopEmitter so unit tests and dev workflows
// stay deterministic.
//
// Note: refund_charge / create_payout / handle_chargeback (action
// allowlist), verify_webhook_signature (helper), and efi_webhook_received
// (reactor) stay in the legacy adapter.Execute switch path — they are
// NOT resource-typed ensure_/observe_/destroy_ ops, so the Reconciler
// surface does not apply.
func WireReconcilers(a *adapter.Adapter, instanceID string) {
	emitter := newEmitterFromEnv()
	commonOpts := []reconcile.Option{
		reconcile.WithProvider(Provider),
		reconcile.WithEmitter(emitter),
		reconcile.WithInstanceID(instanceID),
	}

	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "charge", "charges",
		newChargeReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_charge",
				"get_charge_status",
				"get_statement",
			),
		)...,
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "due_charge", "due_charges",
		newDueChargeReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_due_charge",
			),
		)...,
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "webhook_subscription", "webhook_subscriptions",
		newWebhookSubscriptionReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"register_webhook_endpoint",
				"unregister_webhook_endpoint",
			),
		)...,
	)
}

// newEmitterFromEnv returns an events.Emitter wired to yggdrasil-core
// when YGGDRASIL_CORE_URL is set, otherwise a NoopEmitter. Env-driven
// keeps the Lego principle: no broker / secret-store / cloud is
// hardcoded; callers point us at any core URL they want. Emission is
// best-effort (see sdk/reconcile.WithEmitter docstring).
func newEmitterFromEnv() events.Emitter {
	if os.Getenv(events.EnvCoreURL) == "" {
		return &events.NoopEmitter{}
	}
	return events.NewHTTPEmitter()
}
