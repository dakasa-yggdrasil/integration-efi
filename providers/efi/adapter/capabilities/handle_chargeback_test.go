package capabilities

import (
	"context"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestHandleChargeback_PassesThroughWithProcessed(t *testing.T) {
	c := &efiapi.EfiClient{}
	got, err := HandleChargeback(context.Background(), c, map[string]any{
		"e2eId":         "E2E-cb-1",
		"chargeback_id": "CB-1",
		"valor":         "10.00",
		"status":        "OPENED",
	})
	if err != nil {
		t.Fatalf("HandleChargeback = %v", err)
	}
	if got["e2eId"] != "E2E-cb-1" {
		t.Fatalf("e2eId = %v", got["e2eId"])
	}
	if got["chargeback_id"] != "CB-1" {
		t.Fatalf("chargeback_id = %v", got["chargeback_id"])
	}
	if got["processed"] != true {
		t.Fatalf("processed = %v, want true", got["processed"])
	}
	if got["status"] != "OPENED" {
		t.Fatalf("status pass-through = %v", got["status"])
	}
}

func TestHandleChargeback_RequiresE2eIdAndChargebackId(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := HandleChargeback(context.Background(), c, map[string]any{"chargeback_id": "CB"})
	if err == nil || !strings.Contains(err.Error(), "e2eId") {
		t.Fatalf("expected e2eId required, got %v", err)
	}
	_, err = HandleChargeback(context.Background(), c, map[string]any{"e2eId": "E"})
	if err == nil || !strings.Contains(err.Error(), "chargeback_id") {
		t.Fatalf("expected chargeback_id required, got %v", err)
	}
}
