package efiapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// Exercise the real capability's v2-to-v3 path: a check only at the original
// reconciler entry would incorrectly permit a fallback send after expiry.
func TestConfidentialMutationGuardRunsAgainBeforeActualV3Fallback(t *testing.T) {
	for _, allowFallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "expires-after-v2", true: "both-sends-authorized"}[allowFallback], func(t *testing.T) {
			var v2, v3, guards atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/oauth/token":
					_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
				case "/v2/webhook/fixed-key":
					v2.Add(1)
					w.WriteHeader(http.StatusNotFound)
				case "/v3/gn/webhook/fixed-key":
					v3.Add(1)
					w.WriteHeader(http.StatusOK)
				default:
					t.Error("unexpected HTTP path")
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()
			ctx := efiapi.WithMutationGuard(efiapi.WithConfidentialDiagnostics(context.Background()), func() error {
				if guards.Add(1) > 1 && !allowFallback {
					return errors.New("expired private authorization")
				}
				return nil
			})
			client, err := efiapi.NewEfiClientWithContext(ctx, config.Config{BaseURL: srv.URL}, nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := capabilities.EnsureWebhookSubscription(ctx, client, map[string]any{"chave": "fixed-key", "webhook_url": "https://receiver.invalid/base", "skip_mtls_validation": false})
			if guards.Load() != 2 || v2.Load() != 1 {
				t.Fatalf("each-send guard was not used: guards=%d v2=%d", guards.Load(), v2.Load())
			}
			if allowFallback {
				if err != nil || v3.Load() != 1 || out["endpoint"] != "v3" {
					t.Fatalf("authorized fallback failed: v3=%d err=%v", v3.Load(), err)
				}
			} else if !errors.Is(err, efiapi.ErrConfidentialMutationRefused) || v3.Load() != 0 || out != nil {
				t.Fatalf("expired fallback escaped: v3=%d err=%v", v3.Load(), err)
			}
		})
	}
}
