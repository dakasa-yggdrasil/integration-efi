package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"
	"go.uber.org/zap"

	model "github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
)

// ExecuteHandler is the SDK handler that routes operations to the
// adapter. Production wiring (v2.2.0+): inbound envelopes route through
// reconcile.Dispatch FIRST — activating §6.5 mutation event
// auto-emission via the WireReconcilers-installed dispatch table.
// Operations not registered with a Reconciler (refund_charge,
// create_payout, handle_chargeback, verify_webhook_signature,
// efi_webhook_received) fall back to the legacy adapter.Execute switch.
//
// The fallback is the Option B "hybrid bridge" pattern mirroring
// integration-slack and integration-stripe v2.x: the legacy Execute
// path stays the source of truth for non-resource operations, while
// the §6.5-emitting reconcile path handles the three resource types
// (charge, due_charge, webhook_subscription).
func ExecuteHandler(logger *zap.Logger, a *sdkadapter.Adapter) Handler {
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		var envelope struct {
			Operation  string `json:"operation"`
			Capability string `json:"capability,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return failure("bad_request", err, logger)
		}
		operation := adapter.NormalizeExecuteOperation(envelope.Operation, envelope.Capability)
		capability := adapter.NormalizeExecuteCapability(envelope.Capability, operation)
		if !adapter.SupportsExecuteCapability(capability) {
			return failure("unsupported_capability", fmt.Errorf("unsupported capability %q", envelope.Capability), logger)
		}
		var req model.AdapterExecuteIntegrationRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			return failure("bad_request", err, logger)
		}

		// Bridge: rebuild the SDK-shaped envelope so reconcile.Dispatch
		// can route by operation. The SDK's executeRequest reads
		// {operation, capability, instance_id, input} at the top level
		// — the wire-level integration.instance.name MUST be lifted so
		// §6.5 emission metadata + reconciler-side instance lookup work.
		sdkDelivery, sdkErr := buildSDKDelivery(d, req)
		if sdkErr != nil {
			return failure("bad_request", sdkErr, logger)
		}
		body, _, dispatchErr := reconcile.Dispatch(ctx, a, sdkDelivery)
		if dispatchErr == nil {
			// SDK reconcile path succeeded — re-wrap the raw observed
			// JSON in the adapter's rpcResponse envelope so callers see
			// the same {ok,data} shape they always have.
			var out map[string]any
			if err := json.Unmarshal(body, &out); err != nil {
				// Body was non-object (e.g. observe items wrapped).
				out = map[string]any{"raw": json.RawMessage(body)}
			}
			return success(model.AdapterExecuteIntegrationResponse{
				Operation:  req.Operation,
				Capability: req.Capability,
				Status:     "ok",
				Output:     out,
				Metadata:   map[string]any{"provider": adapter.Provider, "via": "reconcile"},
			})
		}
		if !isUnsupportedReconcileOp(dispatchErr) {
			return failure("execute_failed", dispatchErr, logger)
		}

		// Operation has no Reconciler — fall back to the legacy switch
		// (action helpers, verify_webhook_signature, reactor replay).
		response, err := adapter.Execute(req)
		if err != nil {
			return failure("execute_failed", err, logger)
		}
		return success(response)
	}
}

// buildSDKDelivery rewrites the inbound wire body into the shape the
// SDK reconcile dispatch expects: {operation, capability, instance_id,
// idempotency, input}. The instance_id is lifted from
// integration.instance.name AND injected into input.instance_id so the
// in-tree reconciler dispatch helpers can extract it per-call. The
// full Integration shape is stuffed under input._integration so the
// reconciler bridge can rebuild the per-request Execute envelope
// (auth + credentials + instance spec) for the legacy switch path.
func buildSDKDelivery(d rpc.Delivery, req model.AdapterExecuteIntegrationRequest) (rpc.Delivery, error) {
	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	instanceID := strings.TrimSpace(req.Integration.Instance.Name)
	if instanceID != "" {
		if _, present := input["instance_id"]; !present {
			input["instance_id"] = instanceID
		}
	}
	// Forward the full Integration shape so defaultDispatch in
	// providers/efi/adapter/reconcile.go can rebuild the per-request
	// Execute envelope (credentials + instance config) used by the
	// legacy switch.
	if _, present := input["_integration"]; !present {
		b, err := json.Marshal(req.Integration)
		if err == nil {
			var as map[string]any
			if jerr := json.Unmarshal(b, &as); jerr == nil {
				input["_integration"] = as
			}
		}
	}
	idempotency, _ := req.Metadata["idempotency"].(string)
	sdkBody, err := json.Marshal(map[string]any{
		"operation":   req.Operation,
		"capability":  req.Capability,
		"instance_id": instanceID,
		"idempotency": idempotency,
		"input":       input,
	})
	if err != nil {
		return rpc.Delivery{}, err
	}
	return rpc.Delivery{Body: sdkBody, ContentType: d.ContentType}, nil
}

// isUnsupportedReconcileOp matches the SDK's "unsupported operation"
// signal so the bridge falls back to the legacy switch instead of
// surfacing the error to callers. Also covers the "no registered
// Reconciler" sentinel emitted when reconcile.Dispatch is called
// before WireReconcilers — defensive only; main() wires before
// ExecuteHandler in normal flow.
func isUnsupportedReconcileOp(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "reconcile: unsupported operation") ||
		strings.Contains(msg, "reconcile: adapter has no registered Reconciler")
}
