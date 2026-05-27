//go:build integration

// Package integration_tests holds the build-tag-gated EFI sandbox
// E2E suite. Skip unless RUN_INTEGRATION_TESTS=true is set in the
// environment.
package integration_tests

import (
	"context"
	"os"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestSandbox_CreateChargeAgainstHomologation(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("set RUN_INTEGRATION_TESTS=true to run")
	}
	cfg := config.Config{
		ClientKeyID:     os.Getenv("EFI_API_CLIENT_KEY_ID"),
		ClientSecret:    os.Getenv("EFI_API_CLIENT_SECRET"),
		CertificatePath: os.Getenv("EFI_CERTIFICATE"),
		BaseURL:         "https://pix-h.api.efipay.com.br",
		MTLSEnabled:     true,
	}
	tlsConfig, err := adapter.LoadTLSConfig(cfg)
	if err != nil {
		t.Fatalf("LoadTLSConfig: %v", err)
	}
	c, err := efiapi.NewEfiClient(cfg, tlsConfig)
	if err != nil {
		t.Fatalf("NewEfiClient: %v", err)
	}
	got, err := capabilities.CreateCharge(context.Background(), c, map[string]any{
		"valor": map[string]any{"original": "1.00"},
		"chave": os.Getenv("EFI_TEST_PIX_KEY"),
	})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if got["txid"] == nil {
		t.Fatalf("expected txid in response, got %v", got)
	}
	if got["status"] != "ATIVA" {
		t.Fatalf("status = %v, want ATIVA", got["status"])
	}
}
