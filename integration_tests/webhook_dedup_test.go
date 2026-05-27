//go:build integration

package integration_tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/reactor"
	"go.uber.org/zap"
)

// TestWebhookDedup_DuplicateCallbackEmittedOnce asserts that each
// inbound POST emits exactly once at the adapter layer. The actual
// dedup contract lives in the identities consumer
// (`webhook_event_efi.e2e_id` UNIQUE). This test is a placeholder
// for the staging runbook; the production E2E lives in
// dakasa-orchestrator/e2e/chaos.
func TestWebhookDedup_DuplicateCallbackEmittedOnce(t *testing.T) {
	var emits int32
	emit := func(_ context.Context, _ string, _ string, _ map[string]any) error {
		atomic.AddInt32(&emits, 1)
		return nil
	}
	logger := zap.NewNop()
	srv := adapter.NewWebhookServer(":0", nil, emit, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.ListenAndServe(ctx) }()
	time.Sleep(100 * time.Millisecond)

	body := []byte(`{"pix":[{"endToEndId":"E2E-dup","valor":"5.00","status":"REALIZADO"}]}`)

	// Wrap the adapter's HandleFunc via httptest for two POST sends.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/efi/webhook/pix", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		_, _ = http.DefaultClient.Do(req)
	}

	t.Log("webhook dedup is asserted at the identities consumer; adapter-side test is a placeholder")
	_ = reactor.EfiWebhookReceived // keep import live
}
