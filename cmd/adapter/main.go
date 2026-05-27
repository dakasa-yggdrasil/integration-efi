// Command integration-efi runs the EFI/Pix Yggdrasil adapter.
//
// At startup:
//
//   - The SDK adapter binds describe + execute handlers under the
//     transport selected by YGGDRASIL_TRANSPORT (http_json default,
//     amqp when set).
//   - A health server listens on HEALTHCHECK_PORT (default 8080) for
//     /healthz, /readyz, and /metrics.
//   - The webhook listener wiring lands in Task 27.
//
// Graceful shutdown on SIGINT/SIGTERM via adapter.WithSignalHandler.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/message"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	a := adapter.New(adapter.Config{
		Provider:        ad.Provider,
		IntegrationType: ad.IntegrationType,
		Version:         ad.AdapterVersion,
		DefaultTimeout:  30 * time.Second,
		Concurrency:     5,
	}).
		Register("describe", message.DescribeHandler(logger)).
		Register("execute", message.ExecuteHandler(logger))

	switch transport := strings.ToLower(strings.TrimSpace(os.Getenv("YGGDRASIL_TRANSPORT"))); transport {
	case "", "http", "http_json":
		addr := ":" + envOrDefault("ADAPTER_PORT", "8081")
		a.ListenHTTP(addr)
		logger.Info("integration-efi adapter starting on HTTP", zap.String("addr", addr))
	case "amqp", "rabbitmq":
		brokerURL := strings.TrimSpace(os.Getenv("BROKER_URL"))
		if brokerURL == "" {
			logger.Fatal("YGGDRASIL_TRANSPORT=amqp but BROKER_URL is empty")
		}
		a.ListenAMQP(brokerURL)
		logger.Info("integration-efi adapter starting on AMQP")
	default:
		logger.Fatal("unsupported YGGDRASIL_TRANSPORT", zap.String("value", transport))
	}

	healthSrv := newHealthServer()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("health server", zap.Error(err))
		}
	}()

	// Webhook listener wiring lands in Task 27.

	ctx := adapter.WithSignalHandler(context.Background())
	if err := a.Run(ctx); err != nil {
		logger.Fatal("adapter run", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Warn("shutdown health server", zap.Error(err))
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
