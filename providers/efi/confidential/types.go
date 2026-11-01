// Package confidential defines the optional two-way sealed webhook wire format.
// It contains no credentials, provider client, SDK transport or persistent state.
package confidential

import "encoding/json"

const (
	SchemaVersion      = 1
	Algorithm          = "nacl-box-seal-anonymous-v1"
	MaxInputPlaintext  = 8 * 1024
	MaxOutputPlaintext = 512 * 1024
	MaxPublicMetadata  = 4096
	KeyTagDomain       = "efi.webhook.confidential.key.v1"
	URLTagDomain       = "efi.webhook.confidential.url.v1"
)

type RuntimePublic struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Algorithm      string `json:"algorithm"`
	AdapterVersion string `json:"adapterVersion"`
	KeyID          string `json:"keyID"`
	PublicKey      string `json:"publicKey"`
}

// Input is the ONLY member of the execute input in confidential mode.
type Input struct {
	ConfidentialInput InputEnvelope `json:"confidential_input"`
}
type InputEnvelope struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Algorithm      string `json:"algorithm"`
	RecipientKeyID string `json:"recipientKeyID"`
	Ciphertext     string `json:"ciphertext"`
}

// Binding claims are compared to the immutable expected attempt by the receiver
// operator. Core Metadata does not authenticate caller-supplied source claims.
type Binding struct {
	SourceRoot              string `json:"source_root"`
	ReleaseSHA256           string `json:"release_sha256"`
	InvocationID            string `json:"invocation_id"`
	Mode                    string `json:"mode"`
	WorkflowID              string `json:"workflow_id"`
	WorkflowVersion         int    `json:"workflow_version"`
	StepID                  string `json:"step_id"`
	Operation               string `json:"operation"`
	Purpose                 string `json:"purpose"`
	InstanceName            string `json:"instance_name"`
	ReceiverKeyID           string `json:"receiver_key_id"`
	ReceiverPublicKeySHA256 string `json:"receiver_public_key_sha256"`
	ResultPublicKeySHA256   string `json:"result_public_key_sha256"`
	ComparisonSecretSHA256  string `json:"comparison_secret_sha256"`
	AuditResourceID         string `json:"audit_resource_id"`
	IssuedAt                string `json:"issued_at"`
	ExpiresAt               string `json:"expires_at"`
}

// Parameters are encrypted before the request reaches HTTP/Core/RabbitMQ. The
// same complete request shape is used for every purpose, with before_state empty
// outside ensure. The public proof never contains these private strings.
type Parameters struct {
	Key                string `json:"chave"`
	CanonicalBase      string `json:"canonical_base"`
	Start              string `json:"inicio"`
	End                string `json:"fim"`
	Page               int    `json:"page"`
	PageSize           int    `json:"page_size"`
	BeforeState        string `json:"before_state"`
	SkipMTLSValidation bool   `json:"skip_mtls_validation"`
}

type Intent struct {
	SchemaVersion    int        `json:"schemaVersion"`
	Binding          Binding    `json:"binding"`
	Parameters       Parameters `json:"parameters"`
	ResultPublicKey  string     `json:"result_public_key"`
	ComparisonSecret string     `json:"comparison_secret"`
}
type Request struct {
	Intent
	IntentSHA256 string `json:"intent_sha256"`
}

type Pagination struct {
	Page     int `json:"paginaAtual"`
	PageSize int `json:"itensPorPagina"`
	Pages    int `json:"quantidadeDePaginas"`
	Total    int `json:"quantidadeTotalDeItens"`
}
type PageParameters struct {
	Start      string     `json:"inicio"`
	End        string     `json:"fim"`
	Pagination Pagination `json:"paginacao"`
}
type PageProvenance struct {
	SchemaVersion int  `json:"schemaVersion"`
	Validated     bool `json:"validated"`
	Complete      bool `json:"complete"`
}
type Row struct {
	KeyTag        string `json:"key_tag"`
	URLTag        string `json:"url_tag"`
	CreationValid bool   `json:"creation_valid"`
}

// Kind-specific fields are omitted, not invented for another kind. List must
// include a nonnil Rows slice even when empty. URLTag is NEVER an ensure endpoint.
type Proof struct {
	SchemaVersion  int             `json:"schemaVersion"`
	Kind           string          `json:"kind"`
	Outcome        string          `json:"outcome"`
	Provider       string          `json:"provider,omitempty"`
	Cursor         *string         `json:"cursor,omitempty"`
	Parameters     *PageParameters `json:"parametros,omitempty"`
	PageProvenance *PageProvenance `json:"page_provenance,omitempty"`
	Rows           *[]Row          `json:"rows,omitempty"`
	KeyTag         string          `json:"key_tag,omitempty"`
	URLTag         string          `json:"url_tag,omitempty"`
	CreationValid  *bool           `json:"creation_valid,omitempty"`
	Ensured        *bool           `json:"ensured,omitempty"`
	Noop           *bool           `json:"noop,omitempty"`
	Endpoint       string          `json:"endpoint,omitempty"`
}
type Privacy struct {
	SchemaVersion int    `json:"schemaVersion"`
	Algorithm     string `json:"algorithm"`
	Ciphertext    string `json:"ciphertext"`
}
type Output struct {
	ID      string  `json:"id,omitempty"`
	Privacy Privacy `json:"privacy"`
	Proof   Proof   `json:"proof"`
}
type PlainOutput struct {
	SchemaVersion     int             `json:"schemaVersion"`
	Binding           Binding         `json:"binding"`
	IntentSHA256      string          `json:"intent_sha256"`
	LegacyOutput      json.RawMessage `json:"legacy_output"`
	GuardObservation  json.RawMessage `json:"guard_observation,omitempty"`
	GuardConfirmation json.RawMessage `json:"guard_confirmation,omitempty"`
}
