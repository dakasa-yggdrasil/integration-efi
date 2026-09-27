package message

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"

	model "github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
)

func TestBuildSDKDeliveryDoesNotMutateLegacyFallbackInput(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"operation": adapter.OperationObserveAutomaticWebhookReadiness,
		"input": map[string]any{
			"expected_dns_name": "nlb.example.net",
		},
		"integration": map[string]any{
			"instance": map[string]any{"name": "efi-dakasa-production"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var req model.AdapterExecuteIntegrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	wantLegacyInput := map[string]any{"expected_dns_name": "nlb.example.net"}

	delivery, err := buildSDKDelivery(rpc.Delivery{Body: body, ContentType: "application/json"}, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.Input, wantLegacyInput) {
		t.Fatalf("legacy fallback input was mutated: %#v", req.Input)
	}

	var bridged struct {
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(delivery.Body, &bridged); err != nil {
		t.Fatal(err)
	}
	if bridged.Input["expected_dns_name"] != "nlb.example.net" ||
		bridged.Input["instance_id"] != "efi-dakasa-production" ||
		bridged.Input["_integration"] == nil || len(bridged.Input) != 3 {
		t.Fatalf("SDK bridge input = %#v", bridged.Input)
	}
}

func TestBuildSDKDeliveryKeepsNilLegacyFallbackInput(t *testing.T) {
	req := model.AdapterExecuteIntegrationRequest{}
	if _, err := buildSDKDelivery(rpc.Delivery{}, req); err != nil {
		t.Fatal(err)
	}
	if req.Input != nil {
		t.Fatalf("nil legacy fallback input was mutated: %#v", req.Input)
	}
}

func TestBuildSDKDeliveryReplacesCallerSuppliedBridgeMetadata(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"operation": adapter.OperationEnsureCharge,
		"input": map[string]any{
			"txid":         "tx-1",
			"instance_id":  "attacker-instance",
			"_integration": map[string]any{"instance": map[string]any{"name": "attacker-instance"}},
		},
		"integration": map[string]any{
			"instance": map[string]any{"name": "efi-dakasa-production"},
			"instance_spec": map[string]any{
				"config": map[string]any{"base_url": "https://pix.api.efipay.com.br"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var req model.AdapterExecuteIntegrationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	wantLegacyInput := map[string]any{
		"txid":         "tx-1",
		"instance_id":  "attacker-instance",
		"_integration": map[string]any{"instance": map[string]any{"name": "attacker-instance"}},
	}

	delivery, err := buildSDKDelivery(rpc.Delivery{Body: body, ContentType: "application/json"}, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.Input, wantLegacyInput) {
		t.Fatalf("legacy fallback input was mutated: %#v", req.Input)
	}

	var bridged struct {
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(delivery.Body, &bridged); err != nil {
		t.Fatal(err)
	}
	trusted, ok := bridged.Input["_integration"].(map[string]any)
	if !ok {
		t.Fatalf("SDK bridge integration = %#v", bridged.Input["_integration"])
	}
	instance, ok := trusted["instance"].(map[string]any)
	if !ok || instance["name"] != "efi-dakasa-production" ||
		bridged.Input["instance_id"] != "efi-dakasa-production" {
		t.Fatalf("SDK bridge accepted caller metadata: %#v", bridged.Input)
	}
}
