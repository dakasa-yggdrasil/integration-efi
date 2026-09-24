// Command integration-efi runs the EFI/Pix Yggdrasil adapter.
//
// At startup:
//
//   - The SDK adapter binds describe + execute handlers under the
//     transport selected by YGGDRASIL_TRANSPORT (http_json default,
//     amqp when set).
//   - A health server listens on HEALTHCHECK_PORT (default 8080) for
//     /healthz, /readyz, and /metrics.
//   - The EFI mTLS material in the EFI_* env is loaded once, so an
//     unusable certificate fails the boot instead of the first call.
//
// The adapter runs no inbound webhook listener. EFI delivers Pix
// callbacks to the webhook_url registered through
// ensure_webhook_subscription, which is served by another service.
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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"

	ad "github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/message"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	// OTel tracer provider — installed globally so efiapi.do() picks it up.
	tp, err := newTracerProvider(context.Background())
	if err != nil {
		logger.Warn("OTel tracer init failed; spans will be no-op", zap.Error(err))
	}
	if tp != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tp.Shutdown(shutdownCtx)
		}()
	}

	a := adapter.New(adapter.Config{
		Provider:        ad.Provider,
		IntegrationType: ad.IntegrationType,
		Version:         ad.AdapterVersion,
		DefaultTimeout:  30 * time.Second,
		Concurrency:     5,
	})

	// Wire the SDK reconcile dispatch table BEFORE Register so the
	// SDK auto-installs its execute handler. The custom ExecuteHandler
	// below then clobbers that handler (last-write-wins) with the
	// hybrid bridge that routes resource-typed ops through
	// reconcile.Dispatch (activating §6.5 emission) and falls back to
	// the legacy adapter.Execute switch for action helpers + reactor.
	//
	// instanceID is the fallback used when the inbound envelope carries
	// no integration.instance.name; payload-bound values take
	// precedence (integrationFromPayload in reconcile.go).
	instanceID := strings.TrimSpace(os.Getenv("YGGDRASIL_INTEGRATION_INSTANCE_NAME"))
	ad.WireReconcilers(a, instanceID)

	a = a.
		Register("describe", message.DescribeHandler(logger)).
		Register("execute", message.ExecuteHandler(logger, a))

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

	ctx := adapter.WithSignalHandler(context.Background())

	// Execute loads the TLS config again per request (instance config
	// may override the env), so this load only guards the boot.
	if _, err := ad.LoadTLSConfig(config.Load()); err != nil {
		logger.Fatal("load mTLS", zap.Error(err))
	}

	ad.AdapterUp.Set(1)

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

// newTracerProvider wires an OTLP gRPC exporter when
// OTEL_EXPORTER_OTLP_ENDPOINT is set; otherwise returns a no-op
// provider. The global tracer is installed so efiapi.do() picks it
// up automatically.
func newTracerProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, nil
	}
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
	otel.SetTracerProvider(tp)
	return tp, nil
}
