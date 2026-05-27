package adapter

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/reactor"
)

// WebhookServer is the inbound HTTP listener for EFI Pix callbacks.
// mTLS-required when tlsConfig != nil. Routes:
//
//	GET  /efi/webhook/pix  → URL-validation probe (200 OK).
//	POST /efi/webhook/pix  → callback; calls reactor.EfiWebhookReceived
//	                          and returns 202 on first delivery / 500 on emit fail.
type WebhookServer struct {
	addr      string
	tlsConfig *tls.Config
	emit      reactor.EmitFunc
	logger    *zap.Logger
	srv       *http.Server
}

// NewWebhookServer builds a *WebhookServer that will listen on addr
// (e.g. ":9079"). Pass nil tlsConfig for HTTP-only (mock mode).
func NewWebhookServer(addr string, tlsConfig *tls.Config, emit reactor.EmitFunc, logger *zap.Logger) *WebhookServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/efi/webhook/pix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("EFI Pix Webhook is operational"))
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		pixStatus := ""
		if pixSlice, ok := payload["pix"].([]any); ok && len(pixSlice) > 0 {
			if first, ok := pixSlice[0].(map[string]any); ok {
				if s, ok := first["status"].(string); ok {
					pixStatus = s
				}
			}
		}
		got, err := reactor.EfiWebhookReceived(r.Context(), emit, payload)
		if err != nil {
			WebhookReceived.WithLabelValues("emit_failed", pixStatus).Inc()
			logger.Error("efi webhook emit failed", zap.Error(err))
			http.Error(w, "emit failed", http.StatusInternalServerError)
			return
		}
		if emitted, _ := got["emitted"].(bool); !emitted {
			WebhookReceived.WithLabelValues("noop", pixStatus).Inc()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		WebhookReceived.WithLabelValues("received", pixStatus).Inc()
		w.WriteHeader(http.StatusAccepted)
	})

	tlsCopy := tlsConfig
	if tlsCopy != nil {
		tlsCopy.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return &WebhookServer{
		addr:      addr,
		tlsConfig: tlsCopy,
		emit:      emit,
		logger:    logger,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			TLSConfig:         tlsCopy,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
		},
	}
}

// ListenAndServe blocks until ctx is cancelled or the listener fails.
// On ctx cancellation, gracefully shuts down with a 5s deadline.
func (s *WebhookServer) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		var err error
		if s.tlsConfig != nil {
			err = s.srv.ListenAndServeTLS("", "")
		} else {
			err = s.srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(shutdownCtx)
	if err, ok := <-errCh; ok && err != nil {
		return err
	}
	return nil
}
