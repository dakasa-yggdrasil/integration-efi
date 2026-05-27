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
	if AdapterVersion != "1.0.0" {
		t.Fatalf("AdapterVersion = %q, want 1.0.0", AdapterVersion)
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
