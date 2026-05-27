package adapter

import (
	"fmt"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

// Execute is the operation dispatcher. As capabilities land in
// providers/efi/adapter/capabilities, this function gains a case in
// the switch. The bootstrap version returns "not yet implemented" for
// any operation so the ExecuteHandler compiles.
func Execute(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	operation := NormalizeExecuteOperation(req.Operation, req.Capability)
	return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("operation %q not yet implemented", operation)
}
