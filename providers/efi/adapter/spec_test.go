package adapter

import (
	"encoding/json"
	"testing"

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
