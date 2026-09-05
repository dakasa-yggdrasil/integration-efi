package adapter

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/mtls"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
)

var loadSystemCertPool = x509.SystemCertPool

// LoadTLSConfig loads the mTLS *tls.Config from the configured P12
// source. Returns (nil, nil) when EFI_MTLS_ENABLED=false (mock mode).
// Returns an error when MTLSEnabled=true but no cert source is
// configured.
func LoadTLSConfig(cfg config.Config) (*tls.Config, error) {
	if !cfg.MTLSEnabled {
		return nil, nil
	}
	src := mtls.Config{}
	switch {
	case cfg.CertificatePath != "":
		src.Source = mtls.SourceFile
		src.Path = cfg.CertificatePath
	case cfg.CertificateBase64 != "":
		src.Source = mtls.SourceBase64
		src.Base64 = cfg.CertificateBase64
	default:
		return nil, errMissingCert
	}
	tlsConfig, err := mtls.Load(src)
	if err != nil {
		return nil, err
	}
	roots, err := loadSystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system root CAs: %w", err)
	}
	if roots == nil {
		return nil, errors.New("load system root CAs: empty pool")
	}
	tlsConfig.RootCAs = roots
	tlsConfig.MinVersion = tls.VersionTLS12
	return tlsConfig, nil
}

var errMissingCert = mtlsConfigError("EFI_MTLS_ENABLED=true but no EFI_CERTIFICATE or EFI_CERTIFICATE_BASE64 set")

type mtlsConfigError string

func (e mtlsConfigError) Error() string { return string(e) }
