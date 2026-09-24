package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/reactor"
)

// TestDefaultReactorEmit_FailsWithoutSink pins the 2.5.1 behavior: with
// the workflow-run dispatch gone, a non-empty pix array must fail
// instead of reporting emitted=true for an event nothing published.
func TestDefaultReactorEmit_FailsWithoutSink(t *testing.T) {
	in := map[string]any{
		"pix": []any{map[string]any{
			"endToEndId": "E2E-no-sink",
			"valor":      "1.00",
			"status":     "REALIZADO",
		}},
	}
	got, err := reactor.EfiWebhookReceived(context.Background(), DefaultReactorEmit, in)
	if !errors.Is(err, ErrNoReactorSink) {
		t.Fatalf("err = %v, want ErrNoReactorSink", err)
	}
	if got != nil {
		t.Fatalf("output = %v, want nil on failure", got)
	}
}

// TestDefaultReactorEmit_EmptyPixStaysNoop keeps the empty-batch path
// successful: it never reaches the emitter.
func TestDefaultReactorEmit_EmptyPixStaysNoop(t *testing.T) {
	got, err := reactor.EfiWebhookReceived(context.Background(), DefaultReactorEmit, map[string]any{"pix": []any{}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got["emitted"] != false {
		t.Fatalf("emitted = %v, want false", got["emitted"])
	}
}
