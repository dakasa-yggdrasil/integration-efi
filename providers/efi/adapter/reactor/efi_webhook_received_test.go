package reactor

import (
	"context"
	"testing"
)

func TestEfiWebhookReceived_EmitsEnvelopeToIdentitiesQueue(t *testing.T) {
	var publishedTo, publishedRoutingKey string
	var publishedPayload map[string]any

	emit := func(_ context.Context, exchange, routingKey string, payload map[string]any) error {
		publishedTo = exchange
		publishedRoutingKey = routingKey
		publishedPayload = payload
		return nil
	}

	in := map[string]any{
		"pix": []any{map[string]any{
			"endToEndId": "E2E-abc-123",
			"chave":      "user@dakasa.me",
			"valor":      "10.00",
			"status":     "REALIZADO",
			"horario":    "2026-05-26T14:00:00Z",
		}},
	}
	got, err := EfiWebhookReceived(context.Background(), emit, in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got["emitted"] != true {
		t.Fatalf("emitted = %v", got["emitted"])
	}
	if publishedRoutingKey != "identities.efi.pix-receive.q" {
		t.Fatalf("routing_key = %q, want identities.efi.pix-receive.q", publishedRoutingKey)
	}
	if publishedPayload["event"] != "efi.pix.received" {
		t.Fatalf("event = %v", publishedPayload["event"])
	}
	if publishedPayload["e2eId"] != "E2E-abc-123" {
		t.Fatalf("e2eId = %v", publishedPayload["e2eId"])
	}
	if publishedTo != "amq.default" {
		t.Fatalf("exchange = %q, want amq.default", publishedTo)
	}
}

func TestEfiWebhookReceived_EmptyPixArrayReturnsNoop(t *testing.T) {
	emit := func(_ context.Context, _, _ string, _ map[string]any) error {
		t.Fatalf("emit must not be called for empty pix")
		return nil
	}
	got, err := EfiWebhookReceived(context.Background(), emit, map[string]any{"pix": []any{}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got["emitted"] != false {
		t.Fatalf("emitted = %v, want false", got["emitted"])
	}
}
