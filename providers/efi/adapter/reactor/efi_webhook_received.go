// Package reactor implements the inbound webhook-fired capabilities.
// These are NOT user-dispatched through Execute (they would be no-ops
// without the actual webhook delivery) — they're invoked directly by
// the WebhookServer in providers/efi/adapter/webhook_server.go.
package reactor

import (
	"context"
	"fmt"
	"time"
)

// EmitFunc is the dependency-injected emitter. In production this
// posts a `publish_message` workflow run against yggdrasil-core
// (which routes to integration-rabbitmq-runtime). In tests it
// captures the call args.
type EmitFunc func(ctx context.Context, exchange, routingKey string, payload map[string]any) error

// EfiWebhookReceived consumes the JSON body EFI POSTed to our
// /efi/webhook/pix endpoint, extracts the first pix entry, and emits
// a normalized event envelope to the identities consumer queue.
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
