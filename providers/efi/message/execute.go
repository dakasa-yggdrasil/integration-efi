package message

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"

	model "github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
)

// ExecuteHandler is the SDK handler that fans operations out through
// adapter.Execute. Every operation runs through the same envelope —
// the per-capability switch lives in providers/efi/adapter/adapter.go.
func ExecuteHandler(logger *zap.Logger) Handler {
	return func(_ context.Context, d rpc.Delivery) ([]byte, string, error) {
		var envelope struct {
			Operation  string `json:"operation"`
			Capability string `json:"capability,omitempty"`
		}
		if err := json.Unmarshal(d.Body, &envelope); err != nil {
			return failure("bad_request", err, logger)
		}
		operation := adapter.NormalizeExecuteOperation(envelope.Operation, envelope.Capability)
		capability := adapter.NormalizeExecuteCapability(envelope.Capability, operation)
		if !adapter.SupportsExecuteCapability(capability) {
			return failure("unsupported_capability", fmt.Errorf("unsupported capability %q", envelope.Capability), logger)
		}
		var req model.AdapterExecuteIntegrationRequest
		if err := json.Unmarshal(d.Body, &req); err != nil {
			return failure("bad_request", err, logger)
		}
		response, err := adapter.Execute(req)
		if err != nil {
			return failure("execute_failed", err, logger)
		}
		return success(response)
	}
}
