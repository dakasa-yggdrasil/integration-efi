package adapter

import (
	"context"
	"fmt"
	"log"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/reactor"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// DefaultReactorEmit is the EmitFunc installed by main.go after the
// production emit function (Yggdrasil-orchestrator round-trip) has
// been constructed. Replay-via-Execute uses this. Initialized to a
// no-op so tests / unit invocations of Execute do not panic.
var DefaultReactorEmit reactor.EmitFunc = func(_ context.Context, _, _ string, _ map[string]any) error {
	return nil
}

// LegacyDeprecationLogger is the function called the first time a v1.x
// operation name routes through the compat shim. Production wires this
// to a zap.SugaredLogger.Warnf in main.go; tests can override it to
// capture invocations. Default uses log.Printf so the WARN is at least
// visible during dev runs.
//
// The shim is removed in integration-efi v3.0.0 (matching SDK v0.6.0).
var LegacyDeprecationLogger = func(format string, args ...any) {
	log.Printf("WARN "+format, args...)
}

// Execute dispatches one EFI operation. It builds a fresh EfiClient
// per request — the OAuth token is short-lived and an EFI roundtrip
// is cheap. A later iteration could cache by instance-config hash.
//
// Legacy compat (v2.0.0 transition): if the caller passes a v1.x
// operation name listed in LegacyOperationAliases, it is silently
// remapped to the canonical v2.0.0 name and a deprecation WARN is
// emitted via LegacyDeprecationLogger. The shim is removed in v3.0.0.
func Execute(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	operation := NormalizeExecuteOperation(req.Operation, req.Capability)
	if operation == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("operation is required")
	}
	if canonical, legacy := CanonicalOperationFor(operation); legacy {
		LegacyDeprecationLogger(
			"integration-efi: deprecated capability name %q invoked; use %q (v2.0.0 compat shim, removed in v3.0.0)",
			operation, canonical,
		)
		operation = canonical
	}

	cfg := configFromRequest(req)
	tlsConfig, err := LoadTLSConfig(cfg)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("load mTLS: %w", err)
	}
	client, err := efiapi.NewEfiClient(cfg, tlsConfig)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("authenticate to EFI: %w", err)
	}

	ctx := context.Background()
	output := map[string]any{"provider": Provider}

	switch operation {
	case OperationEnsureCharge:
		got, err := capabilities.EnsureCharge(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationEnsureDueCharge:
		got, err := capabilities.EnsureDueCharge(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationObserveCharges:
		got, err := capabilities.ObserveCharges(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationDestroyCharge:
		got, err := capabilities.DestroyCharge(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationRefundCharge:
		got, err := capabilities.RefundCharge(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationCreatePayout:
		got, err := capabilities.CreatePayout(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationHandleChargeback:
		got, err := capabilities.HandleChargeback(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationEnsureWebhookSubscription:
		got, err := capabilities.EnsureWebhookSubscription(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationObserveWebhookSubscriptions:
		got, err := capabilities.ObserveWebhookSubscriptions(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationDestroyWebhookSubscription:
		got, err := capabilities.DestroyWebhookSubscription(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationVerifyWebhookSignature:
		got, err := capabilities.VerifyWebhookSignature(ctx, client, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	case OperationEfiWebhookReceived:
		// In normal flow this is fired by the WebhookServer directly.
		// Replay from a workflow uses DefaultReactorEmit (installed
		// by main.go after the production emit is built).
		got, err := reactor.EfiWebhookReceived(ctx, DefaultReactorEmit, req.Input)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for k, v := range got {
			output[k] = v
		}
	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported operation %q", operation)
	}

	return contract.AdapterExecuteIntegrationResponse{
		Operation:  operation,
		Capability: operation,
		Status:     "ok",
		Output:     output,
		Metadata:   map[string]any{"provider": Provider, "base_url": cfg.BaseURL},
	}, nil
}

// configFromRequest builds a config.Config from the instance manifest
// (instance_spec.credentials/config), falling back to process env vars
// when an instance field is unset.
func configFromRequest(req contract.AdapterExecuteIntegrationRequest) config.Config {
	cfg := config.Load() // env defaults
	creds := req.Integration.InstanceSpec.Credentials
	inst := req.Integration.InstanceSpec.Config

	if v, _ := creds["efi_client_key_id"].(string); v != "" {
		cfg.ClientKeyID = v
	}
	if v, _ := creds["efi_client_secret"].(string); v != "" {
		cfg.ClientSecret = v
	}
	if v, _ := creds["efi_certificate_base64"].(string); v != "" {
		cfg.CertificateBase64 = v
	}
	if v, _ := inst["base_url"].(string); v != "" {
		cfg.BaseURL = v
	}
	if v, ok := inst["mtls_enabled"].(bool); ok {
		cfg.MTLSEnabled = v
	}
	return cfg
}
