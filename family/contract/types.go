// Package contract defines the shared protocol exchanged between Yggdrasil
// orchestrator and the EFI/Pix integration adapter.
//
// The adapter speaks the standard AdapterDescribeResponse /
// AdapterExecuteIntegrationRequest envelopes documented in the
// yggdrasil-core integration framework. Type shapes mirror the generic
// adapter contract used by integration-rabbitmq, integration-grafana,
// and other Yggdrasil adapters.
package contract

import "github.com/google/uuid"

type ManifestSelector struct {
	ManifestID string `json:"manifest_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Version    *int   `json:"version,omitempty"`
}

type ManifestReference struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
}

type IntegrationTypeManifestSpec struct {
	Provider             string                        `json:"provider"`
	FamilyRef            *ManifestSelector             `json:"family_ref,omitempty"`
	ImplementedOperations []string                     `json:"implemented_operations,omitempty"`
	Adapter              IntegrationAdapterSpec        `json:"adapter"`
	Capabilities         []string                      `json:"capabilities"`
	CredentialSchema     IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema       IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes        []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog        []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery            IntegrationDiscoverySpec      `json:"discovery"`
	Normalization        IntegrationNormalizationSpec  `json:"normalization"`
	Execution            IntegrationExecutionSpec      `json:"execution"`
	Extensions           IntegrationExtensionsSpec     `json:"extensions"`
}

type IntegrationAdapterSpec struct {
	Transport      string                  `json:"transport"`
	Version        string                  `json:"version"`
	Queues         IntegrationAdapterQueue `json:"queues,omitempty"`
	Endpoints      IntegrationAdapterRoute `json:"endpoints,omitempty"`
	TimeoutSeconds int                     `json:"timeout_seconds,omitempty"`
}

// IntegrationAdapterRoute carries the HTTP endpoint paths emitted in
// Describe() when the adapter is running with transport=http_json.
type IntegrationAdapterRoute struct {
	Describe string `json:"describe,omitempty"`
	Execute  string `json:"execute,omitempty"`
}

type IntegrationAdapterQueue struct {
	Describe string `json:"describe,omitempty"`
	Discover string `json:"discover,omitempty"`
	Read     string `json:"read,omitempty"`
	Execute  string `json:"execute,omitempty"`
	Sync     string `json:"sync,omitempty"`
	Health   string `json:"health,omitempty"`
}

type IntegrationSchemaSpec struct {
	Mode       string                               `json:"mode"`
	Required   []string                             `json:"required,omitempty"`
	Properties map[string]IntegrationSchemaProperty `json:"properties,omitempty"`
}

type IntegrationSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Default     any    `json:"default,omitempty"`
}

type IntegrationResourceType struct {
	Name             string   `json:"name"`
	CanonicalPrefix  string   `json:"canonical_prefix"`
	IdentityTemplate string   `json:"identity_template"`
	Discoverable     bool     `json:"discoverable"`
	DefaultActions   []string `json:"default_actions"`
}

type IntegrationActionDefinition struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	Idempotent    bool     `json:"idempotent,omitempty"`
	Category      string   `json:"category,omitempty"`
}

type IntegrationDiscoverySpec struct {
	Mode             string `json:"mode"`
	Cursor           string `json:"cursor,omitempty"`
	SupportsWebhooks bool   `json:"supports_webhooks,omitempty"`
}

type IntegrationNormalizationSpec struct {
	ExternalIDPath         string `json:"external_id_path"`
	NamePath               string `json:"name_path,omitempty"`
	OwnerPath              string `json:"owner_path,omitempty"`
	FallbackResourcePrefix string `json:"fallback_resource_prefix"`
}

type IntegrationExecutionSpec struct {
	SupportsDryRun    bool     `json:"supports_dry_run,omitempty"`
	IdempotentActions []string `json:"idempotent_actions,omitempty"`
}

type IntegrationExtensionsSpec struct {
	AllowCustomResourceTypes bool `json:"allow_custom_resource_types,omitempty"`
	AllowCustomActions       bool `json:"allow_custom_actions,omitempty"`
	PreserveRawPayload       bool `json:"preserve_raw_payload,omitempty"`
}

type IntegrationInstanceManifestSpec struct {
	TypeRef     ManifestSelector                 `json:"type_ref"`
	Status      string                           `json:"status,omitempty"`
	Owners      []string                         `json:"owners,omitempty"`
	Credentials map[string]any                   `json:"credentials,omitempty"`
	Config      map[string]any                   `json:"config,omitempty"`
	Discovery   IntegrationInstanceDiscoverySpec `json:"discovery"`
	Execution   IntegrationInstanceExecutionSpec `json:"execution,omitempty"`
}

type IntegrationInstanceDiscoverySpec struct {
	Enabled             bool   `json:"enabled"`
	Mode                string `json:"mode,omitempty"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds,omitempty"`
}

type IntegrationInstanceExecutionSpec struct {
	DefaultDryRun bool `json:"default_dry_run,omitempty"`
	MaxBatchSize  int  `json:"max_batch_size,omitempty"`
}

type AdapterDescribeRequest struct {
	Provider        string `json:"provider"`
	ExpectedVersion string `json:"expected_version,omitempty"`
}

type AdapterDescribeResponse struct {
	Provider         string                        `json:"provider"`
	Adapter          IntegrationAdapterSpec        `json:"adapter"`
	Capabilities     []string                      `json:"capabilities"`
	CredentialSchema IntegrationSchemaSpec         `json:"credential_schema"`
	InstanceSchema   IntegrationSchemaSpec         `json:"instance_schema"`
	ResourceTypes    []IntegrationResourceType     `json:"resource_types"`
	ActionCatalog    []IntegrationActionDefinition `json:"action_catalog,omitempty"`
	Discovery        IntegrationDiscoverySpec      `json:"discovery"`
	Normalization    IntegrationNormalizationSpec  `json:"normalization"`
	Execution        IntegrationExecutionSpec      `json:"execution"`
	Extensions       IntegrationExtensionsSpec     `json:"extensions"`
}

// Execute envelope shared by every operation that runs against a live
// integration instance (used by runtime + topology providers).
type AdapterExecuteIntegrationContext struct {
	Type         ManifestReference               `json:"type"`
	TypeSpec     IntegrationTypeManifestSpec     `json:"type_spec"`
	Instance     ManifestReference               `json:"instance"`
	InstanceSpec IntegrationInstanceManifestSpec `json:"instance_spec"`
}

type AdapterExecuteIntegrationRequest struct {
	Operation   string                           `json:"operation"`
	Capability  string                           `json:"capability,omitempty"`
	Input       map[string]any                   `json:"input,omitempty"`
	Auth        map[string]any                   `json:"auth,omitempty"`
	Metadata    map[string]any                   `json:"metadata,omitempty"`
	Integration AdapterExecuteIntegrationContext `json:"integration"`
}

type AdapterExecuteIntegrationResponse struct {
	Operation  string         `json:"operation,omitempty"`
	Capability string         `json:"capability,omitempty"`
	Status     string         `json:"status,omitempty"`
	Output     any            `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ProductTargetSpec struct {
	Kind                   string           `json:"kind"`
	IntegrationInstanceRef ManifestSelector `json:"integration_instance_ref"`
	Namespace              string           `json:"namespace,omitempty"`
}

type ProductReconcileSpec struct {
	Strategy string `json:"strategy,omitempty"`
	Prune    bool   `json:"prune,omitempty"`
	Wait     bool   `json:"wait,omitempty"`
}

// Generate / Reconcile envelopes used by providers that materialize component
// installations as raw Kubernetes objects (kubernetes provider).
type AdapterGenerateInstallationContext struct {
	Product   ManifestReference `json:"product"`
	Component string            `json:"component"`
	Category  string            `json:"category,omitempty"`
	Class     string            `json:"class,omitempty"`
}

type AdapterGenerateInstallationIntegrationContext struct {
	Type         ManifestReference               `json:"type"`
	TypeSpec     IntegrationTypeManifestSpec     `json:"type_spec"`
	Instance     ManifestReference               `json:"instance"`
	InstanceSpec IntegrationInstanceManifestSpec `json:"instance_spec"`
}

type AdapterGenerateInstallationRequest struct {
	Operation   string                                        `json:"operation"`
	Context     AdapterGenerateInstallationContext            `json:"context"`
	Integration AdapterGenerateInstallationIntegrationContext `json:"integration"`
	Capability  string                                        `json:"capability,omitempty"`
	Input       map[string]any                                `json:"input,omitempty"`
}

type AdapterGenerateInstallationResponse struct {
	Operation string           `json:"operation,omitempty"`
	Objects   []map[string]any `json:"objects"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

// Backwards-compatible aliases preserved while older catalog manifests still
// use the "product" terminology. New providers should use Installation forms.
type AdapterGenerateProductContext = AdapterGenerateInstallationContext
type AdapterGenerateProductIntegrationContext = AdapterGenerateInstallationIntegrationContext
type AdapterGenerateProductRequest = AdapterGenerateInstallationRequest
type AdapterGenerateProductResponse = AdapterGenerateInstallationResponse

type AdapterReconcileInstallationRequest struct {
	Operation   string                                        `json:"operation"`
	Context     AdapterGenerateInstallationContext            `json:"context"`
	Integration AdapterGenerateInstallationIntegrationContext `json:"integration"`
	Capability  string                                        `json:"capability,omitempty"`
	Input       map[string]any                                `json:"input,omitempty"`
	Target      ProductTargetSpec                             `json:"target"`
	Reconcile   ProductReconcileSpec                          `json:"reconcile,omitempty"`
}

type AdapterReconcileInstallationResponse struct {
	Operation string           `json:"operation,omitempty"`
	Mode      string           `json:"mode,omitempty"`
	Objects   []map[string]any `json:"objects,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

type InstallationResourceState struct {
	Kind      string         `json:"kind"`
	Namespace string         `json:"namespace,omitempty"`
	Name      string         `json:"name"`
	Status    string         `json:"status,omitempty"`
	Observed  bool           `json:"observed,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type AdapterDiscoverInstallationStateRequest struct {
	Operation   string                                        `json:"operation"`
	Context     AdapterGenerateInstallationContext            `json:"context"`
	Integration AdapterGenerateInstallationIntegrationContext `json:"integration"`
	Capability  string                                        `json:"capability,omitempty"`
	Input       map[string]any                                `json:"input,omitempty"`
	Target      ProductTargetSpec                             `json:"target"`
}

type AdapterDiscoverInstallationStateResponse struct {
	Operation string                      `json:"operation,omitempty"`
	Status    string                      `json:"status,omitempty"`
	Observed  bool                        `json:"observed,omitempty"`
	Resources []InstallationResourceState `json:"resources,omitempty"`
	Metadata  map[string]any              `json:"metadata,omitempty"`
}
