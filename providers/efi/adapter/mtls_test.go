package adapter

import (
	"path/filepath"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
)

func TestLoadTLSConfig_Disabled_ReturnsNil(t *testing.T) {
	cfg := config.Config{MTLSEnabled: false}
	got, err := LoadTLSConfig(cfg)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Fatalf("got = %v, want nil when MTLSEnabled=false", got)
	}
}

func TestLoadTLSConfig_FromFile_Valid(t *testing.T) {
	cfg := config.Config{
		MTLSEnabled:     true,
		CertificatePath: filepath.Join("testdata", "test.p12"),
	}
	got, err := LoadTLSConfig(cfg)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil {
		t.Fatalf("got = nil, want non-nil *tls.Config")
	}
	if len(got.Certificates) == 0 {
		t.Fatalf("Certificates empty")
	}
}

func TestLoadTLSConfig_EnabledNoSource_Errors(t *testing.T) {
	cfg := config.Config{MTLSEnabled: true} // no path or base64
	_, err := LoadTLSConfig(cfg)
	if err == nil {
		t.Fatalf("expected error when MTLSEnabled=true and no cert source")
	}
}
