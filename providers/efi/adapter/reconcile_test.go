package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"
)

// fakeDispatchTable records every dispatch call and returns canned
// responses. Stand-in for the legacy adapter.Execute switch so the
// reconciler unit tests stay hermetic (no EFI HTTP fixture needed).
type fakeDispatchTable struct {
	calls map[string]reconcilePayload
	resp  map[string]reconcilePayload
	err   map[string]error
}

func newFakeDispatchTable() *fakeDispatchTable {
	return &fakeDispatchTable{
		calls: map[string]reconcilePayload{},
		resp:  map[string]reconcilePayload{},
		err:   map[string]error{},
	}
}

func (f *fakeDispatchTable) dispatch(op string, _ string, in reconcilePayload) (reconcilePayload, error) {
	f.calls[op] = in
	if err, ok := f.err[op]; ok {
		return nil, err
	}
	if resp, ok := f.resp[op]; ok {
		return resp, nil
	}
	return reconcilePayload{"op": op}, nil
}

// TestChargeReconciler_EnsureRoutesEnsureCharge confirms the
// chargeReconciler.Ensure call routes through the ensure_charge
// operation with the desired input preserved.
func TestChargeReconciler_EnsureRoutesEnsureCharge(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationEnsureCharge] = reconcilePayload{
		"txid":   "tx_unit",
		"status": "ATIVA",
	}
	r := &chargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	got, err := r.Ensure(context.Background(), reconcilePayload{
		"valor": map[string]any{"original": "1.00"},
		"chave": "pix@unit.test",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got["txid"] != "tx_unit" {
		t.Errorf("txid = %v, want tx_unit", got["txid"])
	}
	if _, called := fake.calls[OperationEnsureCharge]; !called {
		t.Errorf("ensure_charge not dispatched; calls=%v", fake.calls)
	}
}

// TestChargeReconciler_ObserveRoutesObserveCharges confirms
// observe_charges is invoked with the filter forwarded as input.
func TestChargeReconciler_ObserveRoutesObserveCharges(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationObserveCharges] = reconcilePayload{
		"cobs": []any{
			map[string]any{"txid": "tx1", "status": "ATIVA"},
			map[string]any{"txid": "tx2", "status": "CONCLUIDA"},
		},
	}
	r := &chargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	items, cursor, err := r.Observe(context.Background(), map[string]any{
		"inicio": "2026-05-01T00:00:00Z",
		"fim":    "2026-05-27T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty (no pagination in fixture)", cursor)
	}
	if items[0]["txid"] != "tx1" {
		t.Errorf("items[0].txid = %v, want tx1", items[0]["txid"])
	}
}

// TestChargeReconciler_ObserveSingleTxidWrapsAsItem confirms the
// single-txid response (where the response IS the charge object) is
// wrapped into a one-item slice per the SDK convention.
func TestChargeReconciler_ObserveSingleTxidWrapsAsItem(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationObserveCharges] = reconcilePayload{
		"txid":   "tx_solo",
		"status": "CONCLUIDA",
	}
	r := &chargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	items, _, err := r.Observe(context.Background(), map[string]any{"txid": "tx_solo"})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
	if items[0]["txid"] != "tx_solo" {
		t.Errorf("items[0].txid = %v, want tx_solo", items[0]["txid"])
	}
}

// TestChargeReconciler_DestroyRoutesDestroyCharge confirms the ref
// argument (the txid) is forwarded as input.txid.
func TestChargeReconciler_DestroyRoutesDestroyCharge(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationDestroyCharge] = reconcilePayload{
		"destroyed": true,
		"txid":      "tx_kill",
	}
	r := &chargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	if err := r.Destroy(context.Background(), "tx_kill"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	in, called := fake.calls[OperationDestroyCharge]
	if !called {
		t.Fatalf("destroy_charge not dispatched")
	}
	if in["txid"] != "tx_kill" {
		t.Errorf("forwarded txid = %v, want tx_kill", in["txid"])
	}
}

// TestDueChargeReconciler_EnsureRoutesEnsureDueCharge confirms the cobv
// path: ensure_due_charge is dispatched, not ensure_charge.
func TestDueChargeReconciler_EnsureRoutesEnsureDueCharge(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationEnsureDueCharge] = reconcilePayload{
		"txid":   "tx_cobv",
		"status": "ATIVA",
	}
	r := &dueChargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	_, err := r.Ensure(context.Background(), reconcilePayload{
		"txid":  "tx_cobv",
		"valor": map[string]any{"original": "10.00"},
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, called := fake.calls[OperationEnsureDueCharge]; !called {
		t.Errorf("ensure_due_charge not dispatched; calls=%v", fake.calls)
	}
}

// TestDueChargeReconciler_DestroyReusesDestroyCharge confirms the cobv
// destroy path reuses the immediate-charge destroy contract (same BCB
// PATCH endpoint per spec).
func TestDueChargeReconciler_DestroyReusesDestroyCharge(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationDestroyCharge] = reconcilePayload{"destroyed": true, "txid": "tx_cobv"}
	r := &dueChargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	if err := r.Destroy(context.Background(), "tx_cobv"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, called := fake.calls[OperationDestroyCharge]; !called {
		t.Errorf("destroy_charge not dispatched (cobv reuses cob path); calls=%v", fake.calls)
	}
}

// TestWebhookSubscriptionReconciler_EnsureRoutes confirms the canonical
// ensure_webhook_subscription op is dispatched.
func TestWebhookSubscriptionReconciler_EnsureRoutes(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationEnsureWebhookSubscription] = reconcilePayload{
		"ensured":  true,
		"chave":    "pix@unit.test",
		"endpoint": "v2",
	}
	r := &webhookSubscriptionReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	_, err := r.Ensure(context.Background(), reconcilePayload{
		"chave":       "pix@unit.test",
		"webhook_url": "https://callback.test/efi",
	})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, called := fake.calls[OperationEnsureWebhookSubscription]; !called {
		t.Errorf("ensure_webhook_subscription not dispatched; calls=%v", fake.calls)
	}
}

// TestWebhookSubscriptionReconciler_ObserveExtractsWebhooks confirms
// the {webhooks: [...]} BCB shape is normalized into the SDK items[]
// envelope.
func TestWebhookSubscriptionReconciler_ObserveExtractsWebhooks(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationObserveWebhookSubscriptions] = reconcilePayload{
		"webhooks": []any{
			map[string]any{"chave": "pix1", "webhookUrl": "https://a"},
			map[string]any{"chave": "pix2", "webhookUrl": "https://b"},
		},
	}
	r := &webhookSubscriptionReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	items, _, err := r.Observe(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

// TestWebhookSubscriptionReconciler_DestroyForwardsChave confirms the
// chave (Pix key) is forwarded as the destroy ref.
func TestWebhookSubscriptionReconciler_DestroyForwardsChave(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationDestroyWebhookSubscription] = reconcilePayload{
		"destroyed": true,
		"chave":     "pix@unit.test",
	}
	r := &webhookSubscriptionReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	if err := r.Destroy(context.Background(), "pix@unit.test"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	in := fake.calls[OperationDestroyWebhookSubscription]
	if in["chave"] != "pix@unit.test" {
		t.Errorf("forwarded chave = %v, want pix@unit.test", in["chave"])
	}
}

// TestReconciler_DispatchErrorPropagates confirms a dispatch error
// from the legacy switch bubbles up unchanged through the Reconciler
// surface — callers see the same error as direct Execute() would
// return.
func TestReconciler_DispatchErrorPropagates(t *testing.T) {
	want := errors.New("simulated efi 500")
	fake := newFakeDispatchTable()
	fake.err[OperationEnsureCharge] = want
	r := &chargeReconciler{instanceID: "efi-unit", dispatch: fake.dispatch}

	_, err := r.Ensure(context.Background(), reconcilePayload{})
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("expected dispatch error to propagate, got %v", err)
	}
}

// TestWireReconcilers_RegistersThreeCanonicalTriples drives the adapter
// through the SDK reconcile dispatch path and asserts the canonical
// ensure_/observe_/destroy_ ops for charge, due_charge, and
// webhook_subscription all dispatch — the §6.5 emission path lights up
// once an emitter is wired (via env YGGDRASIL_CORE_URL).
func TestWireReconcilers_RegistersThreeCanonicalTriples(t *testing.T) {
	a := sdkadapter.New(sdkadapter.Config{
		Provider:        Provider,
		IntegrationType: IntegrationType,
		Version:         AdapterVersion,
	})

	// Stub the dispatch table BEFORE WireReconcilers so the
	// instantiated reconcilers route into the fake (the test
	// constructor below mirrors what WireReconcilers does but with
	// a fake dispatch).
	fake := newFakeDispatchTable()
	fake.resp[OperationEnsureCharge] = reconcilePayload{"txid": "tx_e2e", "status": "ATIVA"}
	fake.resp[OperationObserveCharges] = reconcilePayload{
		"cobs": []any{map[string]any{"txid": "tx_e2e"}},
	}
	fake.resp[OperationDestroyCharge] = reconcilePayload{"destroyed": true, "txid": "tx_e2e"}
	fake.resp[OperationEnsureDueCharge] = reconcilePayload{"txid": "tx_cobv", "status": "ATIVA"}
	fake.resp[OperationEnsureWebhookSubscription] = reconcilePayload{"ensured": true, "chave": "pix@e2e"}
	fake.resp[OperationObserveWebhookSubscriptions] = reconcilePayload{
		"webhooks": []any{map[string]any{"chave": "pix@e2e"}},
	}
	fake.resp[OperationDestroyWebhookSubscription] = reconcilePayload{"destroyed": true, "chave": "pix@e2e"}

	cr := &chargeReconciler{instanceID: "efi-e2e", dispatch: fake.dispatch}
	dr := &dueChargeReconciler{instanceID: "efi-e2e", dispatch: fake.dispatch}
	wr := &webhookSubscriptionReconciler{instanceID: "efi-e2e", dispatch: fake.dispatch}

	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "charge", "charges", cr,
		reconcile.WithProvider(Provider),
		reconcile.WithInstanceID("efi-e2e"),
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "due_charge", "due_charges", dr,
		reconcile.WithProvider(Provider),
		reconcile.WithInstanceID("efi-e2e"),
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "webhook_subscription", "webhook_subscriptions", wr,
		reconcile.WithProvider(Provider),
		reconcile.WithInstanceID("efi-e2e"),
	)

	cases := []struct {
		op       string
		input    map[string]any
		mustSee  string
		mustCall string
	}{
		{op: "ensure_charge", input: map[string]any{"valor": map[string]any{"original": "1.00"}, "chave": "pix"}, mustSee: "tx_e2e", mustCall: OperationEnsureCharge},
		{op: "observe_charges", input: map[string]any{"inicio": "x", "fim": "y"}, mustSee: "tx_e2e", mustCall: OperationObserveCharges},
		{op: "destroy_charge", input: map[string]any{"ref": "tx_e2e"}, mustSee: "deleted", mustCall: OperationDestroyCharge},
		{op: "ensure_due_charge", input: map[string]any{"txid": "tx_cobv"}, mustSee: "tx_cobv", mustCall: OperationEnsureDueCharge},
		{op: "observe_due_charges", input: map[string]any{"txid": "tx_cobv"}, mustSee: "tx_e2e", mustCall: OperationObserveCharges},
		{op: "destroy_due_charge", input: map[string]any{"ref": "tx_cobv"}, mustSee: "deleted", mustCall: OperationDestroyCharge},
		{op: "ensure_webhook_subscription", input: map[string]any{"chave": "pix@e2e", "webhook_url": "https://x"}, mustSee: "pix@e2e", mustCall: OperationEnsureWebhookSubscription},
		{op: "observe_webhook_subscriptions", input: map[string]any{}, mustSee: "pix@e2e", mustCall: OperationObserveWebhookSubscriptions},
		{op: "destroy_webhook_subscription", input: map[string]any{"ref": "pix@e2e"}, mustSee: "deleted", mustCall: OperationDestroyWebhookSubscription},
	}

	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"operation":   tc.op,
				"instance_id": "efi-e2e",
				"input":       tc.input,
			})
			resp, _, err := reconcile.Dispatch(context.Background(), a, rpc.Delivery{Body: body})
			if err != nil {
				t.Fatalf("Dispatch(%s): %v", tc.op, err)
			}
			if !strings.Contains(string(resp), tc.mustSee) {
				t.Errorf("Dispatch(%s) body = %s, want substring %q", tc.op, string(resp), tc.mustSee)
			}
			if _, called := fake.calls[tc.mustCall]; !called {
				t.Errorf("Dispatch(%s) did not route to %q; calls=%v", tc.op, tc.mustCall, fake.calls)
			}
			// Reset call book between cases so we can assert per-case.
			delete(fake.calls, tc.mustCall)
		})
	}
}

// TestWireReconcilers_LegacyAliasWARNs locks the WithLegacyNames shim:
// the SDK reconcile dispatch must accept the v1.x names (create_charge,
// register_webhook_endpoint, ...) and route them through the canonical
// handler with a WARN log entry.
func TestWireReconcilers_LegacyAliasWARNs(t *testing.T) {
	a := sdkadapter.New(sdkadapter.Config{
		Provider:        Provider,
		IntegrationType: IntegrationType,
		Version:         AdapterVersion,
	})

	fake := newFakeDispatchTable()
	fake.resp[OperationEnsureCharge] = reconcilePayload{"txid": "tx_legacy", "status": "ATIVA"}

	cr := &chargeReconciler{instanceID: "efi-legacy", dispatch: fake.dispatch}
	var warns int
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "charge", "charges", cr,
		reconcile.WithProvider(Provider),
		reconcile.WithInstanceID("efi-legacy"),
		reconcile.WithLegacyNames("create_charge"),
		reconcile.WithWarnLogger(func(string, ...any) { warns++ }),
	)

	body := []byte(`{"operation":"create_charge","input":{"valor":{"original":"1.00"},"chave":"x"}}`)
	resp, _, err := reconcile.Dispatch(context.Background(), a, rpc.Delivery{Body: body})
	if err != nil {
		t.Fatalf("legacy create_charge dispatch failed: %v", err)
	}
	if !strings.Contains(string(resp), "tx_legacy") {
		t.Fatalf("expected tx_legacy in legacy-shim response, got %s", resp)
	}
	if warns != 1 {
		t.Fatalf("expected 1 WARN entry, got %d", warns)
	}
}

// TestSDKDispatch_DestroyCharge_PreservesReservedKeys verifies that
// SDK v0.8.0's DestroyWithDesired[D] interface forwards the
// bridge-stashed reserved keys (_integration / instance_id) through
// the destroy dispatch path the same way ensure_* and observe_* see
// them. Without DestroyWithDesired the SDK would call the legacy
// Destroy(ctx, ref) and the dispatch helper would receive only
// {"txid": ref} — losing the mTLS cert path / EFI client ID stashed
// under _integration. The fake dispatcher records the desired
// payload it receives so the test can assert reserved keys propagate.
func TestSDKDispatch_DestroyCharge_PreservesReservedKeys(t *testing.T) {
	fake := newFakeDispatchTable()
	fake.resp[OperationDestroyCharge] = reconcilePayload{"destroyed": true, "txid": "tx_with_creds"}

	a := sdkadapter.New(sdkadapter.Config{
		Provider:        Provider,
		IntegrationType: IntegrationType,
		Version:         AdapterVersion,
	})

	cr := &chargeReconciler{instanceID: "efi-e2e", dispatch: fake.dispatch}
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "charge", "charges", cr,
		reconcile.WithProvider(Provider),
		reconcile.WithInstanceID("efi-e2e"),
	)

	// Wire body matches what the bridge produces: _integration stashed
	// alongside operator-supplied input fields.
	body, _ := json.Marshal(map[string]any{
		"operation":   OperationDestroyCharge,
		"instance_id": "efi-e2e",
		"input": map[string]any{
			"ref":         "tx_with_creds",
			"instance_id": "efi-e2e",
			"_integration": map[string]any{
				"instance": map[string]any{"name": "efi-e2e"},
				"instance_spec": map[string]any{
					"credentials": map[string]any{"client_id": "efi_client_canary"},
					"config":      map[string]any{"api_base_url": "https://api.test"},
				},
			},
		},
	})

	if _, _, err := reconcile.Dispatch(context.Background(), a, rpc.Delivery{Body: body}); err != nil {
		t.Fatalf("SDK Dispatch destroy_charge: %v", err)
	}

	got, called := fake.calls[OperationDestroyCharge]
	if !called {
		t.Fatalf("destroy_charge not dispatched through fake")
	}
	// Reserved bridge key must arrive in the dispatch payload —
	// proves DestroyWithDesired forwards the full desired, not just ref.
	integ, ok := got["_integration"].(map[string]any)
	if !ok {
		t.Fatalf("DestroyWithDesired dropped _integration; got %v", got)
	}
	spec, _ := integ["instance_spec"].(map[string]any)
	creds, _ := spec["credentials"].(map[string]any)
	if creds["client_id"] != "efi_client_canary" {
		t.Fatalf("DestroyWithDesired dropped instance credentials; got %v", creds)
	}
	// The destroy key the handler expects must also be present.
	if got["txid"] != "tx_with_creds" {
		t.Fatalf("expected txid forwarded, got %v", got["txid"])
	}
}
