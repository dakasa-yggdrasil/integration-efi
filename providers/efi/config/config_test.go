package config

import (
	"testing"
)

func TestLoad_DefaultsWhenEnvUnset(t *testing.T) {
	t.Setenv("EFI_API_CLIENT_KEY_ID", "")
	t.Setenv("EFI_API_CLIENT_SECRET", "")
	t.Setenv("EFI_MTLS_ENABLED", "")
	t.Setenv("EFI_BASE_URL", "")
	t.Setenv("EFI_WEBHOOK_PORT", "")

	cfg := Load()
	if cfg.BaseURL != "https://pix.api.efipay.com.br" {
		t.Fatalf("BaseURL = %q, want production default", cfg.BaseURL)
	}
	if !cfg.MTLSEnabled {
		t.Fatalf("MTLSEnabled = false, want true (safe default)")
	}
	if cfg.WebhookPort != "9079" {
		t.Fatalf("WebhookPort = %q, want 9079", cfg.WebhookPort)
	}
}

func TestLoad_ReadsAllKnobs(t *testing.T) {
	t.Setenv("EFI_API_CLIENT_KEY_ID", "ckid-test")
	t.Setenv("EFI_API_CLIENT_SECRET", "csec-test")
	t.Setenv("EFI_CERTIFICATE", "/etc/efi/cert.p12")
	t.Setenv("EFI_BASE_URL", "https://pix-h.api.efipay.com.br")
	t.Setenv("EFI_MTLS_ENABLED", "false")
	t.Setenv("EFI_WEBHOOK_PORT", "19079")

	cfg := Load()
	if cfg.ClientKeyID != "ckid-test" {
		t.Fatalf("ClientKeyID = %q", cfg.ClientKeyID)
	}
	if cfg.ClientSecret != "csec-test" {
		t.Fatalf("ClientSecret = %q", cfg.ClientSecret)
	}
	if cfg.CertificatePath != "/etc/efi/cert.p12" {
		t.Fatalf("CertificatePath = %q", cfg.CertificatePath)
	}
	if cfg.BaseURL != "https://pix-h.api.efipay.com.br" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.MTLSEnabled {
		t.Fatalf("MTLSEnabled = true, want false")
	}
	if cfg.WebhookPort != "19079" {
		t.Fatalf("WebhookPort = %q", cfg.WebhookPort)
	}
}
