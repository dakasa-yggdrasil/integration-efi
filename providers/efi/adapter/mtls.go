package adapter

import (
	"crypto/tls"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/mtls"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
)

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
	return mtls.Load(src)
}

var errMissingCert = mtlsConfigError("EFI_MTLS_ENABLED=true but no EFI_CERTIFICATE or EFI_CERTIFICATE_BASE64 set")

type mtlsConfigError string

func (e mtlsConfigError) Error() string { return string(e) }
