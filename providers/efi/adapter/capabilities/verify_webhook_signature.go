package capabilities

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// VerifyWebhookSignature is an internal capability (no HTTP). Given a
// base64-DER-encoded peer certificate (typically captured from the
// inbound webhook TLS handshake), it parses + validates the issuer +
// expiry.
//
// Required input: peer_cert_der (base64-encoded DER bytes).
// Optional:       expected_issuer (string — match against CN).
//
// Returns: { valid, subject, issuer, not_after }.
//
// Idempotent — pure computation.
func VerifyWebhookSignature(_ context.Context, _ *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	peerDER, _ := in["peer_cert_der"].(string)
	if peerDER == "" {
		return nil, fmt.Errorf("verify_webhook_signature: peer_cert_der required")
	}
	derBytes, err := base64.StdEncoding.DecodeString(peerDER)
	if err != nil {
		return nil, fmt.Errorf("verify_webhook_signature: bad base64: %w", err)
	}
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("verify_webhook_signature: parse cert: %w", err)
	}
	expectedIssuer, _ := in["expected_issuer"].(string)
	valid := true
	if expectedIssuer != "" && cert.Issuer.CommonName != expectedIssuer {
		valid = false
	}
	if time.Now().After(cert.NotAfter) {
		valid = false
	}
	return map[string]any{
		"valid":     valid,
		"subject":   cert.Subject.CommonName,
		"issuer":    cert.Issuer.CommonName,
		"not_after": cert.NotAfter.Format(time.RFC3339),
	}, nil
}
