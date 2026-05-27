package efiapi

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics emitted by the efiapi HTTP client. See spec
// Section 9 for the canonical metric list.
var (
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "efi_request_duration_seconds",
		Help:    "Duration of outbound EFI API calls.",
		Buckets: prometheus.DefBuckets,
	}, []string{"op", "status_class"})

	RequestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "efi_request_errors_total",
		Help: "Count of non-2xx EFI responses.",
	}, []string{"op", "status"})

	OAuthRefreshes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "efi_oauth_token_refreshes_total",
		Help: "OAuth token refresh count.",
	}, []string{"result"})

	MTLSFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "efi_mtls_handshake_failures_total",
		Help: "mTLS handshake failures (outbound).",
	})
)

// classifyPath returns a short op label for a given EFI URL path.
// Used for metrics labels. Conservative — falls back to "other" for
// unknown paths to keep cardinality bounded.
func classifyPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/oauth/token"):
		return "oauth"
	case strings.HasPrefix(path, "/v2/cobv"):
		return "create_due_charge"
	case strings.HasPrefix(path, "/v2/cob") && strings.Contains(path, "?"):
		return "get_statement"
	case strings.HasPrefix(path, "/v2/cob"):
		return "cob"
	case strings.HasPrefix(path, "/v2/pix/") && strings.Contains(path, "/devolucao/"):
		return "refund_charge"
	case strings.HasPrefix(path, "/v3/gn/pix/"):
		return "create_payout"
	case strings.HasPrefix(path, "/v2/webhook/") || strings.HasPrefix(path, "/v3/gn/webhook/"):
		return "webhook"
	default:
		return "other"
	}
}
