package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"
	"go.uber.org/zap"

	model "github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/confidential"
)

type executeOptions struct {
	receiver *confidential.Receiver
	now      func() time.Time
}
type ExecuteOption func(*executeOptions)

func WithConfidentialReceiver(receiver *confidential.Receiver) ExecuteOption {
	return func(o *executeOptions) { o.receiver = receiver }
}

// ExecuteHandler is the SDK handler that routes operations to the
// adapter. Production wiring (v2.2.0+): inbound envelopes route through
// reconcile.Dispatch first, except read-only webhook observations whose raw
// provider parameters must survive the generic items/cursor projection. The
// SDK path activates §6.5 mutation event
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
func ExecuteHandler(logger *zap.Logger, a *sdkadapter.Adapter, options ...ExecuteOption) Handler {
	opts := executeOptions{now: time.Now}
	for _, option := range options {
		option(&opts)
	}
	return func(ctx context.Context, d rpc.Delivery) ([]byte, string, error) {
		confidentialMode := confidentialEnvelope(d.Body)
		if confidentialMode && !validConfidentialEnvelope(d.Body) {
			return failure("bad_request", confidential.ErrInvalid, logger)
		}
		var envelope struct {
			Operation  string `json:"operation"`
			Capability string `json:"capability,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			if confidentialMode {
				return failure("bad_request", confidential.ErrInvalid, logger)
			}
			return failure("bad_request", err, logger)
		}
		operation := adapter.NormalizeExecuteOperation(envelope.Operation, envelope.Capability)
		capability := adapter.NormalizeExecuteCapability(envelope.Capability, operation)
		if !adapter.SupportsExecuteCapability(capability) {
			if confidentialMode {
				return failure("unsupported_capability", confidential.ErrInvalid, logger)
			}
			return failure("unsupported_capability", fmt.Errorf("unsupported capability %q", envelope.Capability), logger)
		}
		var req model.AdapterExecuteIntegrationRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			if confidentialMode {
				return failure("bad_request", confidential.ErrInvalid, logger)
			}
			return failure("bad_request", err, logger)
		}
		for key := range req.Input {
			if strings.EqualFold(strings.ReplaceAll(key, "_", ""), "confidentialinput") {
				confidentialMode = true
			}
		}
		if confidentialMode {
			var call *adapter.ConfidentialCall
			var err error
			ctx, call, err = adapter.PrepareConfidential(ctx, req, opts.receiver, opts.now)
			if err != nil {
				return failure("execute_failed", confidential.ErrHeld, logger)
			}
			// This is the only Metadata field buildSDKDelivery consumes. Replace
			// it with a closed token before SDK emission; never forward a caller's
			// arbitrary idempotency/correlation/resource text to the event bridge.
			req.Metadata = map[string]any{"idempotency": call.SDKIdempotency()}
			if operation == adapter.OperationObserveWebhookSubscriptions {
				out, err := adapter.ObserveConfidential(ctx, call)
				if err != nil {
					return failure("execute_failed", confidential.ErrHeld, logger)
				}
				return success(model.AdapterExecuteIntegrationResponse{Operation: req.Operation, Capability: req.Capability, Status: "ok", Output: out, Metadata: map[string]any{"provider": adapter.Provider, "via": "observe"}})
			}
		}
		// This observer's declared contract includes the provider's complete
		// page parameters. The generic SDK Observe interface returns only
		// items/cursor and would discard those parameters. Keep this read-only
		// response intact; mutations still use the event-emitting reconciler.
		if operation == adapter.OperationObserveWebhookSubscriptions && !confidentialMode {
			response, err := adapter.Execute(req)
			if err != nil {
				return failure("execute_failed", err, logger)
			}
			response.Output = webhookObservationOutput(response.Output)
			response.Operation = req.Operation
			response.Capability = req.Capability
			response.Metadata = map[string]any{"provider": adapter.Provider, "via": "observe"}
			return success(response)
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
			if confidentialMode {
				return failure("execute_failed", confidential.ErrHeld, logger)
			}
			return failure("execute_failed", dispatchErr, logger)
		}
		if confidentialMode {
			return failure("execute_failed", confidential.ErrHeld, logger)
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

func confidentialEnvelope(raw []byte) bool {
	// Inspect each ORIGINAL top-level input member separately, so a later
	// duplicate cannot hide an earlier sealed request. Values and unrelated
	// nested member names are not opt-in markers for legacy callers.
	scan := json.NewDecoder(bytes.NewReader(raw))
	first, err := scan.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	for scan.More() {
		token, err := scan.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return false
		}
		if strings.EqualFold(key, "input") {
			var value json.RawMessage
			if scan.Decode(&value) != nil {
				return false
			}
			var input map[string]json.RawMessage
			if json.Unmarshal(value, &input) == nil {
				for name := range input {
					if strings.EqualFold(strings.ReplaceAll(name, "_", ""), "confidentialinput") {
						return true
					}
				}
			}
			continue
		}
		var skip json.RawMessage
		if scan.Decode(&skip) != nil {
			return false
		}
	}
	return false
}
func validConfidentialEnvelope(raw []byte) bool {
	// Check the ORIGINAL bytes, before json.Unmarshal can erase duplicates.
	var generic any
	if confidential.DecodeStrict(raw, &generic) != nil {
		return false
	}
	outer, ok := generic.(map[string]any)
	if !ok {
		return false
	}
	for key := range outer {
		switch key {
		case "operation", "capability", "input", "integration", "metadata", "auth":
		default:
			return false
		}
	}
	_, operation := outer["operation"].(string)
	_, input := outer["input"].(map[string]any)
	return operation && input
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
	// The SDK bridge needs two private fields that are not capability inputs.
	// Copy the caller map before adding them: unsupported reconcile operations
	// fall back to adapter.Execute(req), whose strict input contracts must see
	// the original request unchanged.
	input := make(map[string]any, len(req.Input)+2)
	for key, value := range req.Input {
		if key == "instance_id" || key == "_integration" {
			continue
		}
		input[key] = value
	}
	instanceID := strings.TrimSpace(req.Integration.Instance.Name)
	if instanceID != "" {
		input["instance_id"] = instanceID
	}
	// Forward the full Integration shape so defaultDispatch in
	// providers/efi/adapter/reconcile.go can rebuild the per-request
	// Execute envelope (credentials + instance config) used by the
	// legacy switch.
	b, err := json.Marshal(req.Integration)
	if err != nil {
		return rpc.Delivery{}, fmt.Errorf("encode integration context: %w", err)
	}
	var as map[string]any
	if err := json.Unmarshal(b, &as); err != nil {
		return rpc.Delivery{}, fmt.Errorf("decode integration context: %w", err)
	}
	input["_integration"] = as
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
