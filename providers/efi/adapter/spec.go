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
	// v2.2.0: production runtime migrated to sdk/reconcile.Dispatch
	// via the Option B hybrid bridge — 3 Reconcilers authored (charge,
	// due_charge, webhook_subscription) and §6.5 mutation events emit
	// live for ensure_/destroy_ on those resource types when
	// YGGDRASIL_CORE_URL is wired in cluster.
	AdapterVersion = "2.2.0"

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
	OperationDestroyCharge             = "destroy_charge"
	OperationRefundCharge              = "refund_charge"
	OperationCreatePayout              = "create_payout"
	OperationHandleChargeback          = "handle_chargeback"
	OperationEnsureWebhookSubscription   = "ensure_webhook_subscription"
	OperationObserveWebhookSubscriptions = "observe_webhook_subscriptions"
	OperationDestroyWebhookSubscription  = "destroy_webhook_subscription"
	OperationVerifyWebhookSignature    = "verify_webhook_signature"
	OperationEfiWebhookReceived        = "efi_webhook_received"
)

// SDK-only dispatch operations. The Reconciler[D,O] registration in
// reconcile.go installs these names in the SDK dispatch table so the
// canonical ensure_/observe_/destroy_ triple for the due_charge
// resource type is complete. Their handlers route internally to
// OperationObserveCharges / OperationDestroyCharge — BCB Pix exposes
// cob and cobv records under the same /v2/cob/{txid} GET + PATCH paths.
//
// These are intentionally NOT added to SupportedExecuteOperations (the
// gate the legacy adapter.Execute switch consults): they are valid
// ONLY through the SDK reconcile.Dispatch path. The hybrid bridge in
// providers/efi/message/execute.go routes them via the SDK first; the
// legacy switch never sees them.
const (
	SDKOperationObserveDueCharges = "observe_due_charges"
	SDKOperationDestroyDueCharge  = "destroy_due_charge"
)

// SupportedExecuteOperations is the canonical list of operations
// dispatchable through Execute(). It grows as each capability task lands
// (one entry per capability). Kept in sync with ActionCatalog +
// ResourceTypes via the contractcheck lint.
var SupportedExecuteOperations = []string{
	OperationEnsureCharge,
	OperationEnsureDueCharge,
	OperationObserveCharges,
	OperationDestroyCharge,
	OperationRefundCharge,
	OperationCreatePayout,
	OperationHandleChargeback,
	OperationEnsureWebhookSubscription,
	OperationObserveWebhookSubscriptions,
	OperationDestroyWebhookSubscription,
	OperationVerifyWebhookSignature,
	OperationEfiWebhookReceived,
}

// SDKOnlyOperations names the operations registered ONLY in the SDK
// reconcile.Dispatch table — they are not in SupportedExecuteOperations
// (so the legacy adapter.Execute switch rejects them) but the hybrid
// bridge admits them when reconcile.Dispatch can route them. Used by
// SupportsExecuteCapability so the controllers/message gate doesn't
// reject SDK-handled callers BEFORE the bridge gets a chance to route.
var SDKOnlyOperations = []string{
	SDKOperationObserveDueCharges,
	SDKOperationDestroyDueCharge,
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
				DefaultActions:   []string{OperationEnsureCharge, OperationEnsureDueCharge, OperationObserveCharges, OperationDestroyCharge},
			},
			{
				Name:             "pix_transaction",
				CanonicalPrefix:  "thirdparty.efi.pix",
				IdentityTemplate: "pix.{e2eId}",
				Discoverable:     false,
				DefaultActions:   []string{OperationRefundCharge, OperationCreatePayout, OperationHandleChargeback},
			},
			{
				Name:             "webhook_subscription",
				CanonicalPrefix:  "thirdparty.efi.webhook_subscription",
				IdentityTemplate: "webhook_subscription.{chave}",
				Discoverable:     false,
				DefaultActions: []string{
					OperationEnsureWebhookSubscription,
					OperationObserveWebhookSubscriptions,
					OperationDestroyWebhookSubscription,
					OperationVerifyWebhookSignature,
					OperationEfiWebhookReceived,
				},
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
				Name:          OperationDestroyCharge,
				Description:   "Remove (cancel) a Pix charge by txid. PATCH /v2/cob/{txid} with status=REMOVIDA_PELO_USUARIO_RECEBEDOR per BCB Pix spec. 404 from EFI → already-absent success (idempotent).",
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
				Name:          OperationEnsureWebhookSubscription,
				Description:   "Ensure a Pix webhook subscription exists. PUT /v2/webhook/{chave} (v3 fallback on 404). Idempotent — repeat calls reconcile URL/headers without creating duplicates.",
				ResourceTypes: []string{"webhook_subscription"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationObserveWebhookSubscriptions,
				Description:   "Observe Pix webhook subscriptions. Filter {chave} returns the single subscription (GET /v2/webhook/{chave}); empty filter lists all (GET /v2/webhook). Read-only.",
				ResourceTypes: []string{"webhook_subscription"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationDestroyWebhookSubscription,
				Description:   "Remove a Pix webhook subscription. DELETE /v2/webhook/{chave}. 404 treated as already-absent success (idempotent).",
				ResourceTypes: []string{"webhook_subscription"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationVerifyWebhookSignature,
				Description:   "Verify a peer x509 cert from the inbound webhook handshake. Pure computation.",
				ResourceTypes: []string{"webhook_subscription"},
				Idempotent:    true,
				Category:      "capability",
			},
			{
				Name:          OperationEfiWebhookReceived,
				Description:   "Reactor: handle an inbound EFI Pix webhook callback. Emits to identities.efi.pix-receive.q.",
				ResourceTypes: []string{"webhook_subscription"},
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
				OperationDestroyCharge,
				OperationRefundCharge,
				OperationHandleChargeback,
				OperationEnsureWebhookSubscription,
				OperationObserveWebhookSubscriptions,
				OperationDestroyWebhookSubscription,
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
// SupportedExecuteOperations, in SDKOnlyOperations (handled by the SDK
// reconcile dispatch path), names a legacy alias in
// LegacyOperationAliases, or is blank (operation-only dispatch).
func SupportsExecuteCapability(value string) bool {
	value = strings.TrimSpace(value)
	for _, supported := range SupportedExecuteOperations {
		if value == supported {
			return true
		}
	}
	for _, sdkOnly := range SDKOnlyOperations {
		if value == sdkOnly {
			return true
		}
	}
	if _, legacy := LegacyOperationAliases[value]; legacy {
		return true
	}
	return value == ""
}

// LegacyOperationAliases maps every pre-convention v1.x operation
// name to its v2.0.0 canonical replacement. The Execute switch
// resolves through this table BEFORE the canonical dispatch so any
// caller still publishing the legacy names continues to work for one
// minor cycle.
//
// Each legacy invocation logs a structured "deprecated capability
// name" message via the WARN-shim wrapper installed at the call site
// in adapter.go. The shim is removed in integration-efi v3.0.0,
// matching the SDK v0.6.0 deprecation cadence.
//
// Mirror of yggdrasil-sdk-go v0.5.0 sdk/reconcile.WithLegacyNames —
// the same idea expressed in this adapter's local dispatch path.
var LegacyOperationAliases = map[string]string{
	"create_charge":                OperationEnsureCharge,
	"create_due_charge":            OperationEnsureDueCharge,
	"get_charge_status":            OperationObserveCharges,
	"get_statement":                OperationObserveCharges,
	"register_webhook_endpoint":    OperationEnsureWebhookSubscription,
	"unregister_webhook_endpoint":  OperationDestroyWebhookSubscription,
}

// CanonicalOperationFor returns the v2.0.0 canonical operation name
// for value. If value is already canonical (or unrecognized), it is
// returned unchanged. The second return is true ONLY when value was
// a legacy alias — the caller (Execute) uses that to decide whether
// to log the deprecation WARN.
func CanonicalOperationFor(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if canonical, ok := LegacyOperationAliases[value]; ok {
		return canonical, true
	}
	return value, false
}
