package message

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
)

// TestExecuteHandler_AutomaticWebhookRoutesThroughTheBridgeAndEmits drives
// the production wiring end to end: WireReconcilers, the hybrid bridge,
// the legacy Execute switch, the EFI provider and the Core events API.
func TestExecuteHandler_AutomaticWebhookRoutesThroughTheBridgeAndEmits(t *testing.T) {
	const webhookURL = "https://webhook-pix.dakasa.me/payment/webhook/efi"

	var mu sync.Mutex
	var providerCalls []string
	registered := map[string]string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		providerCalls = append(providerCalls, r.Method+" "+r.URL.Path)
		if r.Header.Get("x-skip-mtls-checking") != "" {
			t.Errorf("x-skip-mtls-checking sent")
		}
		switch r.Method {
		case http.MethodGet:
			url, ok := registered[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"webhookUrl": url})
		case http.MethodPut:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			registered[r.URL.Path] = body["webhookUrl"]
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(provider.Close)

	var eventsMu sync.Mutex
	var posted []map[string]any
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/events" || r.Header.Get("Authorization") != "Bearer event-bearer" {
			t.Errorf("unexpected core call %s %s", r.Method, r.URL.Path)
		}
		var ev map[string]any
		_ = json.NewDecoder(r.Body).Decode(&ev)
		eventsMu.Lock()
		posted = append(posted, ev)
		eventsMu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(core.Close)
	t.Setenv("YGGDRASIL_CORE_URL", core.URL)
	t.Setenv("YGGDRASIL_RUN_TOKEN", "event-bearer")

	a := sdkadapter.New(sdkadapter.Config{Provider: adapter.Provider, IntegrationType: adapter.IntegrationType, Version: adapter.AdapterVersion})
	adapter.WireReconcilers(a, "efi-fallback")
	handler := ExecuteHandler(zap.NewNop(), a)

	integration := map[string]any{
		"instance": map[string]any{"name": "efi-dakasa-production"},
		"instance_spec": map[string]any{
			"credentials": map[string]any{"efi_client_key_id": "k", "efi_client_secret": "s"},
			"config":      map[string]any{"base_url": provider.URL, "mtls_enabled": false},
		},
	}
	call := func(operation string, input map[string]any) rpcResponse {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"operation": operation, "input": input, "integration": integration})
		raw, _, err := handler(context.Background(), rpc.Delivery{Body: body})
		if err != nil {
			t.Fatalf("%s handler error: %v", operation, err)
		}
		var resp rpcResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("%s response: %s", operation, raw)
		}
		return resp
	}

	ensured := call(adapter.OperationEnsureAutomaticWebhook, map[string]any{"kind": "cobr", "webhook_url": webhookURL})
	if !ensured.OK {
		t.Fatalf("ensure failed: %+v", ensured.Error)
	}
	data, _ := json.Marshal(ensured.Data)
	if !strings.Contains(string(data), `"operation":"ensure_automatic_webhook"`) || !strings.Contains(string(data), `"webhook_url":"`+webhookURL+`"`) {
		t.Fatalf("ensure response = %s", data)
	}

	confirmed := call(adapter.OperationObserveAutomaticWebhooks, map[string]any{"kind": "cobr", "expected_webhook_url": webhookURL})
	if !confirmed.OK {
		t.Fatalf("readback gate failed: %+v", confirmed.Error)
	}
	gateMiss := call(adapter.OperationObserveAutomaticWebhooks, map[string]any{"kind": "rec", "expected_webhook_url": webhookURL})
	if gateMiss.OK || gateMiss.Error == nil || !strings.Contains(gateMiss.Error.Message, "registered=false") {
		t.Fatalf("absent rec passed the readback gate: %+v", gateMiss)
	}
	bypass := call(adapter.OperationEnsureAutomaticWebhook, map[string]any{"kind": "rec", "webhook_url": webhookURL, "skip_mtls_validation": true})
	if bypass.OK {
		t.Fatal("skip_mtls_validation was accepted through the bridge")
	}
	destroy := call("destroy_automatic_webhook", map[string]any{"kind": "rec"})
	if destroy.OK || destroy.Error == nil || destroy.Error.Code != "unsupported_capability" {
		t.Fatalf("destroy_automatic_webhook was not refused: %+v", destroy)
	}

	mu.Lock()
	gotCalls := strings.Join(providerCalls, ",")
	mu.Unlock()
	want := "GET /v2/webhookcobr,PUT /v2/webhookcobr,GET /v2/webhookcobr,GET /v2/webhookcobr,GET /v2/webhookrec"
	if gotCalls != want {
		t.Fatalf("provider calls = %s\nwant %s", gotCalls, want)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if len(posted) != 1 {
		t.Fatalf("core events = %d, want 1: %v", len(posted), posted)
	}
	ev := posted[0]
	if ev["event_type"] != "efi.automatic_webhook.ensured" || ev["resource_id"] != "webhookcobr" || ev["instance_id"] != "efi-dakasa-production" {
		t.Fatalf("event = %v", ev)
	}
}
