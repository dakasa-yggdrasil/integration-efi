package adapter

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"os"
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

func TestLoadTLSConfig_FromBase64_UsesSystemRootsAndTLS12(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "test.p12"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	wantRoots := x509.NewCertPool()
	originalLoadSystemCertPool := loadSystemCertPool
	loadSystemCertPool = func() (*x509.CertPool, error) { return wantRoots, nil }
	t.Cleanup(func() { loadSystemCertPool = originalLoadSystemCertPool })

	got, err := LoadTLSConfig(config.Config{
		MTLSEnabled:       true,
		CertificateBase64: base64.StdEncoding.EncodeToString(raw),
	})
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}
	if got.RootCAs != wantRoots {
		t.Fatal("RootCAs did not use the system certificate pool")
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", got.MinVersion)
	}
}

func TestLoadTLSConfig_FromFile_Valid(t *testing.T) {
	wantRoots := x509.NewCertPool()
	originalLoadSystemCertPool := loadSystemCertPool
	loadSystemCertPool = func() (*x509.CertPool, error) { return wantRoots, nil }
	t.Cleanup(func() { loadSystemCertPool = originalLoadSystemCertPool })

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
	if got.RootCAs != wantRoots {
		t.Fatalf("RootCAs did not use the system certificate pool")
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", got.MinVersion)
	}
}

func TestLoadTLSConfig_SystemRootsError(t *testing.T) {
	originalLoadSystemCertPool := loadSystemCertPool
	loadSystemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { loadSystemCertPool = originalLoadSystemCertPool })

	cfg := config.Config{
		MTLSEnabled:     true,
		CertificatePath: filepath.Join("testdata", "test.p12"),
	}
	_, err := LoadTLSConfig(cfg)
	if err == nil || err.Error() != "load system root CAs: boom" {
		t.Fatalf("err = %v, want wrapped system root error", err)
	}
}

func TestLoadTLSConfig_NilSystemRootsErrors(t *testing.T) {
	originalLoadSystemCertPool := loadSystemCertPool
	loadSystemCertPool = func() (*x509.CertPool, error) { return nil, nil }
	t.Cleanup(func() { loadSystemCertPool = originalLoadSystemCertPool })

	cfg := config.Config{
		MTLSEnabled:     true,
		CertificatePath: filepath.Join("testdata", "test.p12"),
	}
	_, err := LoadTLSConfig(cfg)
	if err == nil || err.Error() != "load system root CAs: empty pool" {
		t.Fatalf("err = %v, want empty system root pool error", err)
	}
}

func TestLoadTLSConfig_EnabledNoSource_Errors(t *testing.T) {
	cfg := config.Config{MTLSEnabled: true} // no path or base64
	_, err := LoadTLSConfig(cfg)
	if err == nil {
		t.Fatalf("expected error when MTLSEnabled=true and no cert source")
	}
}
