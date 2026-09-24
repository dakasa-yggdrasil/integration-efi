package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/events"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

const testAutomaticWebhookURL = "https://webhook-pix.dakasa.me/payment/webhook/efi"

type recordingEmitter struct {
	mu     sync.Mutex
	events []events.MutationEvent
	err    error
}

func (r *recordingEmitter) Emit(_ context.Context, e events.MutationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return r.err
}

func (r *recordingEmitter) recorded() []events.MutationEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]events.MutationEvent(nil), r.events...)
}

// useAutomaticWebhookEvents swaps the sink for one test and restores it.
func useAutomaticWebhookEvents(t *testing.T, emitter events.Emitter, warn func(string, ...any)) {
	t.Helper()
	automaticWebhookEventsMu.Lock()
	previous := automaticWebhookEvents
	automaticWebhookEvents = automaticWebhookEventSink{emitter: emitter, instanceID: "efi-fallback", warn: warn}
	automaticWebhookEventsMu.Unlock()
	t.Cleanup(func() {
		automaticWebhookEventsMu.Lock()
		automaticWebhookEvents = previous
		automaticWebhookEventsMu.Unlock()
	})
}

// newAutomaticWebhookProvider serves OAuth plus an in-memory
// /v2/webhookrec and /v2/webhookcobr, and fails the test on any other path.
func newAutomaticWebhookProvider(t *testing.T, putStatus int) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	registered := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		if r.URL.Path != "/v2/webhookrec" && r.URL.Path != "/v2/webhookcobr" {
			t.Errorf("unexpected provider call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		if r.Header.Get("x-skip-mtls-checking") != "" {
			t.Errorf("x-skip-mtls-checking sent on %s %s", r.Method, r.URL.Path)
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			url, ok := registered[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"webhookUrl": url})
		case http.MethodPut:
			if putStatus != 0 {
				w.WriteHeader(putStatus)
				return
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			registered[r.URL.Path] = body["webhookUrl"]
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Errorf("unexpected method %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func automaticWebhookRequest(srvURL, operation string, input map[string]any) contract.AdapterExecuteIntegrationRequest {
	return contract.AdapterExecuteIntegrationRequest{
		Operation: operation,
		Input:     input,
		Metadata:  map[string]any{"idempotency": "run-42-register-rec"},
		Integration: contract.AdapterExecuteIntegrationContext{
			Instance: contract.ManifestReference{Name: "efi-dakasa-production"},
			InstanceSpec: contract.IntegrationInstanceManifestSpec{
				Credentials: map[string]any{"efi_client_key_id": "k", "efi_client_secret": "s"},
				Config:      map[string]any{"base_url": srvURL, "mtls_enabled": false},
			},
		},
	}
}

func TestExecute_EnsureAutomaticWebhookEmitsOneEnsuredEvent(t *testing.T) {
	srv := newAutomaticWebhookProvider(t, 0)
	emitter := &recordingEmitter{}
	useAutomaticWebhookEvents(t, emitter, func(string, ...any) { t.Error("unexpected WARN") })

	resp, err := Execute(automaticWebhookRequest(srv.URL, OperationEnsureAutomaticWebhook, map[string]any{
		"kind":        "rec",
		"webhook_url": testAutomaticWebhookURL,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Operation != OperationEnsureAutomaticWebhook || resp.Status != "ok" {
		t.Fatalf("response = %+v", resp)
	}
	out := resp.Output.(map[string]any)
	if out["registered"] != true || out["webhook_url"] != testAutomaticWebhookURL || out["changed"] != true {
		t.Fatalf("output = %v", out)
	}

	got := emitter.recorded()
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	ev := got[0]
	if ev.EventType != "efi.automatic_webhook.ensured" || ev.Provider != "efi" || ev.Resource != "automatic_webhook" || ev.Verb != events.VerbEnsured {
		t.Fatalf("event identity = %+v", ev)
	}
	if ev.ResourceID != "webhookrec" || ev.InstanceID != "efi-dakasa-production" || ev.Idempotency != "run-42-register-rec" {
		t.Fatalf("event scope = %+v", ev)
	}
	var observed map[string]any
	if err := json.Unmarshal(ev.Observed, &observed); err != nil || observed["webhook_url"] != testAutomaticWebhookURL {
		t.Fatalf("observed = %s (%v)", ev.Observed, err)
	}
	if strings.Contains(string(ev.Observed), `"s"`) || strings.Contains(string(ev.Observed), "efi_client_secret") {
		t.Fatalf("event carried credentials: %s", ev.Observed)
	}
}

func TestExecute_EnsureAutomaticWebhookEmitFailureIsWarnedNotFatal(t *testing.T) {
	srv := newAutomaticWebhookProvider(t, 0)
	emitter := &recordingEmitter{err: errors.New("core refused the event")}
	var warns []string
	useAutomaticWebhookEvents(t, emitter, func(format string, args ...any) {
		warns = append(warns, fmt.Sprintf(format, args...))
	})

	req := automaticWebhookRequest(srv.URL, OperationEnsureAutomaticWebhook, map[string]any{"kind": "cobr", "webhook_url": testAutomaticWebhookURL})
	req.Metadata = nil
	if _, err := Execute(req); err != nil {
		t.Fatalf("a refused event must not fail the proven mutation: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "efi.automatic_webhook.ensured") || !strings.Contains(warns[0], "webhookcobr") {
		t.Fatalf("warns = %v", warns)
	}
	ev := emitter.recorded()[0]
	if !strings.HasPrefix(ev.Idempotency, "efi.automatic_webhook.ensured.webhookcobr.") {
		t.Fatalf("synthesized idempotency = %q", ev.Idempotency)
	}
}

func TestExecute_AutomaticWebhookFailuresAndReadsEmitNothing(t *testing.T) {
	emitter := &recordingEmitter{}
	useAutomaticWebhookEvents(t, emitter, func(string, ...any) {})

	rejecting := newAutomaticWebhookProvider(t, http.StatusBadRequest)
	if _, err := Execute(automaticWebhookRequest(rejecting.URL, OperationEnsureAutomaticWebhook, map[string]any{"kind": "rec", "webhook_url": testAutomaticWebhookURL})); err == nil {
		t.Fatal("provider rejection was reported as success")
	}

	srv := newAutomaticWebhookProvider(t, 0)
	out, err := Execute(automaticWebhookRequest(srv.URL, OperationObserveAutomaticWebhooks, map[string]any{"kind": "rec"}))
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if out.Output.(map[string]any)["registered"] != false {
		t.Fatalf("observe output = %v", out.Output)
	}
	if n := len(emitter.recorded()); n != 0 {
		t.Fatalf("failed ensure or observe emitted %d events", n)
	}
}

func TestDescribe_AutomaticWebhookHasEnsureAndObserveOnly(t *testing.T) {
	desc := Describe()
	var found bool
	for _, rt := range desc.ResourceTypes {
		if rt.Name != ResourceAutomaticWebhook {
			continue
		}
		found = true
		want := []string{OperationEnsureAutomaticWebhook, OperationObserveAutomaticWebhooks}
		if fmt.Sprint(rt.DefaultActions) != fmt.Sprint(want) {
			t.Fatalf("automatic_webhook default actions = %v, want %v", rt.DefaultActions, want)
		}
		if rt.IdentityTemplate != "automatic_webhook.{kind}" {
			t.Fatalf("identity template = %q", rt.IdentityTemplate)
		}
	}
	if !found {
		t.Fatal("automatic_webhook resource type missing")
	}
	raw, _ := json.Marshal(desc)
	if strings.Contains(string(raw), "destroy_automatic_webhook") {
		t.Fatal("describe advertises a destroy for automatic webhooks")
	}
	if SupportsExecuteCapability("destroy_automatic_webhook") {
		t.Fatal("destroy_automatic_webhook is dispatchable")
	}
	for _, op := range []string{OperationEnsureAutomaticWebhook, OperationObserveAutomaticWebhooks} {
		if !SupportsExecuteCapability(op) {
			t.Fatalf("%s is not dispatchable", op)
		}
	}
}

// TestWireReconcilers_LeavesAutomaticWebhooksToTheLegacySwitch proves the
// SDK dispatch table has no automatic_webhook entry at all, so the hybrid
// bridge falls back to Execute for ensure/observe and nothing can route a
// destroy.
func TestWireReconcilers_LeavesAutomaticWebhooksToTheLegacySwitch(t *testing.T) {
	a := sdkadapter.New(sdkadapter.Config{Provider: Provider, IntegrationType: IntegrationType, Version: AdapterVersion})
	previous := currentAutomaticWebhookEvents()
	t.Cleanup(func() {
		automaticWebhookEventsMu.Lock()
		automaticWebhookEvents = previous
		automaticWebhookEventsMu.Unlock()
	})
	WireReconcilers(a, "efi-unit")
	if got := currentAutomaticWebhookEvents(); got.instanceID != "efi-unit" || got.emitter == nil {
		t.Fatalf("WireReconcilers did not configure the automatic webhook events: %+v", got)
	}
	for _, op := range []string{OperationEnsureAutomaticWebhook, OperationObserveAutomaticWebhooks, "destroy_automatic_webhook"} {
		body, _ := json.Marshal(map[string]any{"operation": op, "input": map[string]any{"kind": "rec"}})
		_, _, err := reconcile.Dispatch(context.Background(), a, rpc.Delivery{Body: body})
		if err == nil || !strings.Contains(err.Error(), "reconcile: unsupported operation") {
			t.Fatalf("%s via SDK dispatch: err=%v, want unsupported", op, err)
		}
	}
}
