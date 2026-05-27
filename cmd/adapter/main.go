// Command integration-efi runs the EFI/Pix Yggdrasil adapter.
//
// At startup:
//
//   - The SDK adapter binds describe + execute handlers under the
//     transport selected by YGGDRASIL_TRANSPORT (http_json default,
//     amqp when set).
//   - A health server listens on HEALTHCHECK_PORT (default 8080) for
//     /healthz, /readyz, and /metrics.
//   - A webhook listener listens on EFI_WEBHOOK_PORT (default 9079)
//     for inbound EFI Pix callbacks (mTLS required when an mTLS
//     cert is loaded).
//
// Graceful shutdown on SIGINT/SIGTERM via adapter.WithSignalHandler.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/reactor"
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

	// Webhook server wiring — fires reactor.EfiWebhookReceived on
	// inbound POST /efi/webhook/pix and emits the event envelope to
	// the identities consumer queue via integration-rabbitmq-runtime
	// publish_message.
	ctx := adapter.WithSignalHandler(context.Background())

	cfg := config.Load()
	tlsConfig, err := ad.LoadTLSConfig(cfg)
	if err != nil {
		logger.Fatal("load mTLS", zap.Error(err))
	}
	emit := newProductionEmitFunc(
		os.Getenv("YGGDRASIL_CORE_BASE_URL"),
		os.Getenv("YGGDRASIL_WORKFLOW_RUN_TOKEN"),
		logger,
	)
	ad.DefaultReactorEmit = emit
	webhookSrv := ad.NewWebhookServer(":"+cfg.WebhookPort, tlsConfig, emit, logger)
	go func() {
		if err := webhookSrv.ListenAndServe(ctx); err != nil {
			logger.Fatal("webhook server", zap.Error(err))
		}
	}()

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

// newProductionEmitFunc returns an EmitFunc that POSTs a
// publish_message workflow run to the orchestrator, which routes to
// integration-rabbitmq-runtime. We do NOT publish AMQP directly from
// this adapter — that responsibility lives with the rabbit adapter.
func newProductionEmitFunc(coreBaseURL, token string, logger *zap.Logger) reactor.EmitFunc {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	return func(ctx context.Context, exchange, routingKey string, payload map[string]any) error {
		if strings.TrimSpace(coreBaseURL) == "" {
			logger.Warn("YGGDRASIL_CORE_BASE_URL unset; skipping emit (dev mode)",
				zap.String("routing_key", routingKey))
			return nil
		}
		body := map[string]any{
			"workflow": map[string]any{"name": "publish-message", "namespace": "global"},
			"inputs": map[string]any{
				"integration_instance_ref": map[string]any{"namespace": "global", "name": "rabbitmq-runtime"},
				"capability":               "publish_message",
				"input": map[string]any{
					"exchange":    exchange,
					"routing_key": routingKey,
					"payload":     payload,
				},
			},
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreBaseURL+"/api/v1/workflow-runs", bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("yggdrasil publish_message failed (status=%d): %s", resp.StatusCode, string(b))
		}
		return nil
	}
}
