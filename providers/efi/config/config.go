// Package config loads the EFI/Pix adapter env knobs into a typed
// Config struct. Defaults match the spec: production base URL,
// mTLS-enabled, port 9079 for webhooks.
package config

import (
	"os"
	"strings"
)

// Config carries all env-driven runtime knobs. Populated by Load() at
// startup; mutated by configFromRequest() per-execute when an
// instance manifest carries overrides.
type Config struct {
	ClientKeyID       string
	ClientSecret      string
	CertificatePath   string
	CertificateBase64 string
	BaseURL           string
	MTLSEnabled       bool
	WebhookPort       string
}

// Load reads EFI_* env vars and returns the populated Config. Missing
// values fall back to safe production defaults (mTLS on, prod base
// URL, port 9079).
func Load() Config {
	return Config{
		ClientKeyID:       getEnv("EFI_API_CLIENT_KEY_ID"),
		ClientSecret:      getEnv("EFI_API_CLIENT_SECRET"),
		CertificatePath:   getEnv("EFI_CERTIFICATE"),
		CertificateBase64: getEnv("EFI_CERTIFICATE_BASE64"),
		BaseURL:           getEnvDefault("EFI_BASE_URL", "https://pix.api.efipay.com.br"),
		MTLSEnabled:       getEnvBool("EFI_MTLS_ENABLED", true),
		WebhookPort:       getEnvDefault("EFI_WEBHOOK_PORT", "9079"),
	}
}

func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func getEnvDefault(key, fallback string) string {
	if v := getEnv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	switch strings.ToLower(getEnv(key)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}
