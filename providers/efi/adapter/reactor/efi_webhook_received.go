// Package reactor implements the inbound webhook-fired capabilities.
// The adapter runs no inbound listener, so the only caller left is
// Execute, which passes adapter.DefaultReactorEmit as the emitter.
package reactor

import (
	"context"
	"fmt"
	"time"
)

// EmitFunc is the dependency-injected emitter. The adapter wires no
// production sink (adapter.DefaultReactorEmit fails). In tests it
// captures the call args.
type EmitFunc func(ctx context.Context, exchange, routingKey string, payload map[string]any) error

// EfiWebhookReceived consumes an EFI Pix callback body, extracts the
// first pix entry, and emits a normalized event envelope addressed to
// the identities consumer queue.
//
// Returns `{ emitted: true, e2eId }` on success; `{ emitted: false }`
// on empty pix arrays (URL-validation probe, occasional empty batches).
//
// Note: this adapter does NOT dedup. The identities consumer enforces
// `webhook_event_efi.e2e_id` UNIQUE.
func EfiWebhookReceived(ctx context.Context, emit EmitFunc, in map[string]any) (map[string]any, error) {
	pix, _ := in["pix"].([]any)
	if len(pix) == 0 {
		return map[string]any{"emitted": false}, nil
	}
	first, _ := pix[0].(map[string]any)
	if first == nil {
		return nil, fmt.Errorf("efi_webhook_received: pix[0] must be an object")
	}
	envelope := map[string]any{
		"event":       "efi.pix.received",
		"e2eId":       first["endToEndId"],
		"txid":        first["txid"],
		"valor":       first["valor"],
		"status":      first["status"],
		"chave":       first["chave"],
		"horario":     first["horario"],
		"devolucoes":  first["devolucoes"],
		"received_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := emit(ctx, "amq.default", "identities.efi.pix-receive.q", envelope); err != nil {
		return nil, fmt.Errorf("efi_webhook_received: emit: %w", err)
	}
	return map[string]any{"emitted": true, "e2eId": first["endToEndId"]}, nil
}
