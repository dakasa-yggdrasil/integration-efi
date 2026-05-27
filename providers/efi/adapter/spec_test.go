package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	if AdapterVersion != "2.0.0" {
		t.Fatalf("AdapterVersion = %q, want 2.0.0", AdapterVersion)
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
	if len(entries) != 11 {
		t.Fatalf("expected 11 capability YAMLs, got %d", len(entries))
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
