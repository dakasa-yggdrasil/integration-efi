package adapter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Adapter-level metrics (the per-call HTTP duration / error /
// oauth metrics live in efiapi/metrics.go).
var (
	AdapterUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "efi_adapter_up",
		Help: "1 when adapter healthy.",
	})
)
