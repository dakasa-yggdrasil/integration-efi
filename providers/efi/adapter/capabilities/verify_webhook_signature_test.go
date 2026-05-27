package capabilities

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func loadTestDER(t *testing.T) string {
	t.Helper()
	// testdata lives under providers/efi/adapter/testdata relative to
	// this package's working dir at run time (the capabilities pkg
	// shares the parent's testdata).
	der, err := os.ReadFile(filepath.Join("..", "testdata", "test.der"))
	if err != nil {
		t.Fatalf("read test.der: %v", err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func TestVerifyWebhookSignature_ParsesValidDER(t *testing.T) {
	c := &efiapi.EfiClient{}
	got, err := VerifyWebhookSignature(context.Background(), c, map[string]any{
		"peer_cert_der": loadTestDER(t),
	})
	if err != nil {
		t.Fatalf("VerifyWebhookSignature = %v", err)
	}
	if got["subject"] != "efi-test" {
		t.Fatalf("subject = %v, want efi-test", got["subject"])
	}
	if got["valid"] != true {
		t.Fatalf("valid = %v, want true (no expected_issuer + not yet expired)", got["valid"])
	}
}

func TestVerifyWebhookSignature_DetectsIssuerMismatch(t *testing.T) {
	c := &efiapi.EfiClient{}
	got, err := VerifyWebhookSignature(context.Background(), c, map[string]any{
		"peer_cert_der":   loadTestDER(t),
		"expected_issuer": "some-other-issuer",
	})
	if err != nil {
		t.Fatalf("VerifyWebhookSignature = %v", err)
	}
	if got["valid"] != false {
		t.Fatalf("valid = %v, want false on issuer mismatch", got["valid"])
	}
}

func TestVerifyWebhookSignature_RequiresPeerCertDER(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := VerifyWebhookSignature(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "peer_cert_der") {
		t.Fatalf("expected peer_cert_der required, got %v", err)
	}
}

func TestVerifyWebhookSignature_RejectsBadBase64(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := VerifyWebhookSignature(context.Background(), c, map[string]any{
		"peer_cert_der": "!!not-base64!!",
	})
	if err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("expected base64 error, got %v", err)
	}
}
