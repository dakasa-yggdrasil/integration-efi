package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dakasa-yggdrasil/integration-efi/pkg/contractcheck"
)

func TestDescribeHTTP(t *testing.T) {
	t.Setenv("YGGDRASIL_TRANSPORT", "http")
	response := Describe()
	if response.Provider != Provider {
		t.Fatalf("provider = %q, want %q", response.Provider, Provider)
	}
	if response.Adapter.Transport != "http_json" {
		t.Fatalf("transport = %q, want http_json", response.Adapter.Transport)
	}
	if response.Adapter.Endpoints.Describe != "/rpc/describe" {
		t.Fatalf("describe endpoint = %q, want /rpc/describe", response.Adapter.Endpoints.Describe)
	}
	if response.Adapter.Endpoints.Execute != "/rpc/execute" {
		t.Fatalf("execute endpoint = %q, want /rpc/execute", response.Adapter.Endpoints.Execute)
	}
}

func TestProviderConstants(t *testing.T) {
	if Provider != "efi" {
		t.Fatalf("Provider = %q, want efi", Provider)
	}
	if IntegrationType != "efi" {
		t.Fatalf("IntegrationType = %q, want efi", IntegrationType)
	}
	if AdapterVersion != "2.3.1" {
		t.Fatalf("AdapterVersion = %q, want 2.3.1", AdapterVersion)
	}
}

// TestManifestCapabilityYAMLsParse asserts the manifest/capabilities/
// directory holds exactly 11 YAMLs (one per supported operation) and
// that each parses + has a `name` key. This is the cheapest possible
// smoke against the YAML wire shape.
func TestManifestCapabilityYAMLsParse(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "manifest", "capabilities")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 12 {
		t.Fatalf("expected 12 capability YAMLs, got %d", len(entries))
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if doc["name"] == nil {
			t.Errorf("%s missing 'name'", e.Name())
		}
	}
}

// TestIntegrationTypeManifestJSON_IsValid asserts the
// integration_type manifest exists, parses, and carries the expected
// name = "efi".
func TestIntegrationTypeManifestJSON_IsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "manifest", "integration_type.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["name"] != "efi" {
		t.Fatalf("name = %v", doc["name"])
	}
}

// TestSpec_EnsureChargeReplaceCreateCharge asserts the v2.0.0 rename
// landed in the action_catalog: ensure_charge is present and the
// pre-convention create_charge is gone.
func TestSpec_EnsureChargeReplaceCreateCharge(t *testing.T) {
	desc := Describe()
	names := map[string]bool{}
	for _, a := range desc.ActionCatalog {
		names[a.Name] = true
	}
	if !names["ensure_charge"] {
		t.Error("expected ensure_charge in action_catalog")
	}
	if names["create_charge"] {
		t.Error("create_charge must be removed from action_catalog (renamed to ensure_charge)")
	}
}

// TestSpec_EnsureDueChargeReplaceCreateDueCharge asserts the
// create_due_charge → ensure_due_charge rename landed.
func TestSpec_EnsureDueChargeReplaceCreateDueCharge(t *testing.T) {
	desc := Describe()
	names := map[string]bool{}
	for _, a := range desc.ActionCatalog {
		names[a.Name] = true
	}
	if !names["ensure_due_charge"] {
		t.Error("expected ensure_due_charge")
	}
	if names["create_due_charge"] {
		t.Error("create_due_charge must be removed")
	}
}

// TestSpec_ObserveChargesMergesStatusAndStatement asserts the merge
// landed: observe_charges is the single canonical read op, and the
// pre-convention get_charge_status + get_statement are both gone.
func TestSpec_ObserveChargesMergesStatusAndStatement(t *testing.T) {
	desc := Describe()
	names := map[string]bool{}
	for _, a := range desc.ActionCatalog {
		names[a.Name] = true
	}
	if !names["observe_charges"] {
		t.Error("expected observe_charges")
	}
	if names["get_charge_status"] {
		t.Error("get_charge_status must be merged into observe_charges")
	}
	if names["get_statement"] {
		t.Error("get_statement must be merged into observe_charges")
	}
}

// TestSpec_WebhookSubscriptionTripleExists asserts the v2.0.0
// webhook lifecycle triple landed: ensure_/observe_/destroy_ for
// webhook_subscription, and the v1.x register_/unregister_ pair is
// gone.
func TestSpec_WebhookSubscriptionTripleExists(t *testing.T) {
	desc := Describe()
	names := map[string]bool{}
	for _, a := range desc.ActionCatalog {
		names[a.Name] = true
	}
	wantPresent := []string{"ensure_webhook_subscription", "observe_webhook_subscriptions", "destroy_webhook_subscription"}
	wantAbsent := []string{"register_webhook_endpoint", "unregister_webhook_endpoint"}

	for _, n := range wantPresent {
		if !names[n] {
			t.Errorf("expected %q present", n)
		}
	}
	for _, n := range wantAbsent {
		if names[n] {
			t.Errorf("expected %q removed", n)
		}
	}
}

// TestSpec_WebhookSubscriptionResourceTypeDeclared asserts the
// webhook_subscription resource_type carries the canonical triple in
// the first three DefaultActions slots (the convention's
// "default_actions starts with ensure_/observe_/destroy_").
func TestSpec_WebhookSubscriptionResourceTypeDeclared(t *testing.T) {
	desc := Describe()
	for _, rt := range desc.ResourceTypes {
		if rt.Name == "webhook_subscription" {
			want := []string{"ensure_webhook_subscription", "observe_webhook_subscriptions", "destroy_webhook_subscription"}
			if len(rt.DefaultActions) < 3 || !reflect.DeepEqual(rt.DefaultActions[:3], want) {
				t.Errorf("expected canonical triple in DefaultActions[:3], got %v", rt.DefaultActions)
			}
			return
		}
	}
	t.Fatal("webhook_subscription resource_type missing")
}

// TestSpec_DestroyChargeExists asserts the new destroy_charge entry
// landed so the charge resource_type now has the full canonical
// ensure_/observe_/destroy_ triple.
func TestSpec_DestroyChargeExists(t *testing.T) {
	desc := Describe()
	for _, a := range desc.ActionCatalog {
		if a.Name == "destroy_charge" {
			return
		}
	}
	t.Fatal("expected destroy_charge in action_catalog")
}

// TestDescribeContractAligned guards against the drift pattern that has
// bitten integration-aws and integration-grafana: SupportedExecuteOperations,
// ResourceTypes and ActionCatalog must agree shape-by-shape. The lint
// runs over the JSON-projection of Describe() so it exercises the same
// wire shape the core would receive.
func TestDescribeContractAligned(t *testing.T) {
	raw, err := json.Marshal(Describe())
	if err != nil {
		t.Fatalf("marshal describe: %v", err)
	}
	var snapshot contractcheck.DescribeResponse
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal describe into contractcheck: %v", err)
	}
	if err := contractcheck.LintDescribeContract(snapshot, SupportedExecuteOperations); err != nil {
		t.Fatalf("describe contract drift:\n%s", err.Error())
	}
}
