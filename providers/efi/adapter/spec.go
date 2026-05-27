// Package adapter implements the EFI/Pix Yggdrasil adapter. It exposes
// the Describe() contract advertised on the integration_type manifest
// and an Execute() dispatcher that routes operations to capability
// implementations under providers/efi/adapter/capabilities.
package adapter

import (
	"os"
	"strings"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

const (
	// Provider is the integration_family identifier. Validated by the
	// describe handshake in providers/efi/message/describe.go.
	Provider = "efi"

	// IntegrationType is the integration_type ID — single-provider, so
	// equal to Provider. The SDK derives queue prefixes from this.
	IntegrationType = "efi"

	// AdapterVersion is the wire-advertised adapter binary version.
	AdapterVersion = "2.0.0"

	// QueueDescribe / QueueExecute are the AMQP queue names used when
	// transport=amqp. http_json mode uses the Endpoints instead.
	QueueDescribe = "yggdrasil.adapter.efi.describe"
	QueueExecute  = "yggdrasil.adapter.efi.execute"
)

// Operation constants are declared here (rather than imported from
// capabilities/) so that spec.go can reference them without creating
// an import cycle: the capabilities subpackage already imports
// `adapter` for EfiClient + DoRaw.
const (
	OperationEnsureCharge              = "ensure_charge"
	OperationEnsureDueCharge           = "ensure_due_charge"
	OperationObserveCharges            = "observe_charges"
	OperationRefundCharge              = "refund_charge"
	OperationCreatePayout              = "create_payout"
	OperationHandleChargeback          = "handle_chargeback"
	OperationRegisterWebhookEndpoint   = "register_webhook_endpoint"
	OperationUnregisterWebhookEndpoint = "unregister_webhook_endpoint"
	OperationVerifyWebhookSignature    = "verify_webhook_signature"
	OperationEfiWebhookReceived        = "efi_webhook_received"
)

// SupportedExecuteOperations is the canonical list of operations
// dispatchable through Execute(). It grows as each capability task lands
// (one entry per capability). Kept in sync with ActionCatalog +
// ResourceTypes via the contractcheck lint.
var SupportedExecuteOperations = []string{
	OperationEnsureCharge,
	OperationEnsureDueCharge,
	OperationObserveCharges,
	OperationRefundCharge,
	OperationCreatePayout,
	OperationHandleChargeback,
	OperationRegisterWebhookEndpoint,
	OperationUnregisterWebhookEndpoint,
	OperationVerifyWebhookSignature,
	OperationEfiWebhookReceived,
}

// Describe returns the integration_type manifest the orchestrator
// stores at register-time. The shape is the standard
// AdapterDescribeResponse used by every Yggdrasil adapter.
func Describe() contract.AdapterDescribeResponse {
	transport := strings.ToLower(strings.TrimSpace(os.Getenv("YGGDRASIL_TRANSPORT")))
	if transport == "" {
		transport = "http"
	}
	adapterSpec := contract.IntegrationAdapterSpec{
		Version:        AdapterVersion,
		TimeoutSeconds: 30,
	}
	switch transport {
	case "amqp", "rabbitmq":
		adapterSpec.Transport = "rabbitmq"
		adapterSpec.Queues = contract.IntegrationAdapterQueue{
			Describe: QueueDescribe,
			Execute:  QueueExecute,
		}
	default:
		adapterSpec.Transport = "http_json"
		adapterSpec.Endpoints = contract.IntegrationAdapterRoute{
			Describe: "/rpc/describe",
			Execute:  "/rpc/execute",
		}
	}
	return contract.AdapterDescribeResponse{
		Provider:     Provider,
		Adapter:      adapterSpec,
		Capabilities: []string{"describe", "execute"},
		CredentialSchema: contract.IntegrationSchemaSpec{
			Mode:     "inline",
			Required: []string{"efi_client_key_id", "efi_client_secret"},
			Properties: map[string]contract.IntegrationSchemaProperty{
				"efi_client_key_id": {
					Type:        "string",
					Description: "EFI Pix API client key ID.",
				},
				"efi_client_secret": {
					Type:        "string",
					Description: "EFI Pix API client secret.",
					Secret:      true,
				},
				"efi_certificate_base64": {
					Type:        "string",
					Description: "Base64-encoded P12 mTLS certificate bytes (alternative to mounting EFI_CERTIFICATE file).",
					Secret:      true,
				},
			},
		},
		InstanceSchema: contract.IntegrationSchemaSpec{
			Mode: "inline",
			Properties: map[string]contract.IntegrationSchemaProperty{
				"base_url": {
					Type:        "string",
					Description: "EFI Pix API base URL (pix.api.efipay.com.br or pix-h.api.efipay.com.br for homologation).",
					Default:     "https://pix.api.efipay.com.br",
				},
				"sandbox": {
					Type:        "boolean",
					Description: "Whether this instance points at EFI homologation (pix-h).",
					Default:     false,
				},
				"mtls_enabled": {
					Type:        "boolean",
					Description: "Whether mTLS is enforced for outbound + inbound. Disable only for mock/test instances.",
					Default:     true,
				},
				"webhook_port": {
					Type:        "integer",
					Description: "Port on which the adapter listens for inbound EFI webhook callbacks.",
					Default:     9079,
				},
			},
		},
		ResourceTypes: []contract.IntegrationResourceType{
			{
				Name:             "charge",
				CanonicalPrefix:  "thirdparty.efi.charge",
				IdentityTemplate: "charge.{txid}",
				Discoverable:     false,
				DefaultActions:   []string{OperationEnsureCharge, OperationEnsureDueCharge, OperationObserveCharges},
			},
			{
				Name:             "pix_transaction",
				CanonicalPrefix:  "thirdparty.efi.pix",
				IdentityTemplate: "pix.{e2eId}",
				Discoverable:     false,
				DefaultActions:   []string{OperationRefundCharge, OperationCreatePayout, OperationHandleChargeback},
			},
			{
				Name:             "webhook",
				CanonicalPrefix:  "thirdparty.efi.webhook",
				IdentityTemplate: "webhook.{chave}",
				Discoverable:     false,
				DefaultActions:   []string{OperationRegisterWebhookEndpoint, OperationUnregisterWebhookEndpoint, OperationVerifyWebhookSignature, OperationEfiWebhookReceived},
			},
		},
		ActionCatalog: []contract.IntegrationActionDefinition{
			{
				Name:          OperationEnsureCharge,
				Description:   "Ensure an immediate Pix charge (cob) exists. POST /v2/cob (auto-generated txid) or PUT /v2/cob/{txid} when caller-supplied — repeat PUTs reconcile to the same charge identity, idempotent by txid.",
				ResourceTypes: []string{"charge"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationEnsureDueCharge,
				Description:   "Ensure a due-date Pix charge (cobv) exists. PUT /v2/cobv/{txid}. Idempotent — caller-supplied txid is required (cobv is always upserted).",
				ResourceTypes: []string{"charge"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationObserveCharges,
				Description:   "Observe Pix charges. Filter {txid: X} returns the single charge (GET /v2/cob/{txid}); filter {inicio, fim} returns a paged statement window (GET /v2/cob?inicio=&fim=&page=&page_size=). Read-only.",
				ResourceTypes: []string{"charge"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationRefundCharge,
				Description:   "Refund a Pix transaction. PUT /v2/pix/{e2eId}/devolucao/{id}.",
				ResourceTypes: []string{"pix_transaction"},
				Idempotent:    true, // caller-supplied id is the dedup key
				Category:      "capability",
			},
			{
				Name:          OperationCreatePayout,
				Description:   "Send a Pix payout (envio). PUT /v3/gn/pix/{idEnvio}. IntermediateIrreversible — money movement.",
				ResourceTypes: []string{"pix_transaction"},
				Idempotent:    false, // safety classification — server idempotency exists but caller must treat as opaque
				Category:      "capability",
			},
			{
				Name:          OperationHandleChargeback,
				Description:   "Acknowledge an EFI chargeback. Pass-through, no HTTP call. Idempotent by chargeback_id.",
				ResourceTypes: []string{"pix_transaction"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationRegisterWebhookEndpoint,
				Description:   "Register a Pix webhook URL. PUT /v2/webhook/{chave} (v3 fallback on 404).",
				ResourceTypes: []string{"webhook"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationUnregisterWebhookEndpoint,
				Description:   "Unregister a Pix webhook URL. DELETE /v2/webhook/{chave}. 404 treated as success.",
				ResourceTypes: []string{"webhook"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationVerifyWebhookSignature,
				Description:   "Verify a peer x509 cert from the inbound webhook handshake. Pure computation.",
				ResourceTypes: []string{"webhook"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationEfiWebhookReceived,
				Description:   "Reactor: handle an inbound EFI Pix webhook callback. Emits to identities.efi.pix-receive.q.",
				ResourceTypes: []string{"webhook"},
				Idempotent:    true,
				Category:      "reactor",
			},
		},
		Discovery: contract.IntegrationDiscoverySpec{
			Mode:   "push",
			Cursor: "none",
		},
		Normalization: contract.IntegrationNormalizationSpec{
			ExternalIDPath:         "txid",
			FallbackResourcePrefix: "thirdparty.efi.custom",
		},
		Execution: contract.IntegrationExecutionSpec{
			IdempotentActions: []string{
				OperationEnsureCharge,
				OperationEnsureDueCharge,
				OperationObserveCharges,
				OperationRefundCharge,
				OperationHandleChargeback,
				OperationRegisterWebhookEndpoint,
				OperationUnregisterWebhookEndpoint,
				OperationVerifyWebhookSignature,
				OperationEfiWebhookReceived,
			},
		},
		Extensions: contract.IntegrationExtensionsSpec{
			AllowCustomActions: false,
		},
	}
}

// NormalizeExecuteOperation falls back to capability when operation is
// blank (allowing core-side dispatch envelopes that only set
// capability).
func NormalizeExecuteOperation(operation, capability string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = strings.TrimSpace(capability)
	}
	return operation
}

// NormalizeExecuteCapability mirrors NormalizeExecuteOperation — when
// capability is blank, it inherits the operation value.
func NormalizeExecuteCapability(capability, operation string) string {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		capability = NormalizeExecuteOperation(operation, capability)
	}
	return capability
}

// SupportsExecuteCapability returns true when value names an entry in
// SupportedExecuteOperations (or is blank, allowing operation-only
// dispatch).
func SupportsExecuteCapability(value string) bool {
	value = strings.TrimSpace(value)
	for _, supported := range SupportedExecuteOperations {
		if value == supported {
			return true
		}
	}
	return value == ""
}
