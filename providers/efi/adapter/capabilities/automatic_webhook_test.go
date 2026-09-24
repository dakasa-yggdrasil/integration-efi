package capabilities

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

const canonicalAutomaticWebhookURL = "https://webhook-pix.dakasa.me/payment/webhook/efi"

type recordedRequest struct {
	method     string
	path       string
	rawQuery   string
	body       string
	skipHeader string
}

// fakeAutomaticWebhookProvider emulates the two EFI Pix Automatico webhook
// endpoints. Each path keeps its registered URL. A PUT stores the body's
// webhookUrl unless putStatus or storeOverride says otherwise.
type fakeAutomaticWebhookProvider struct {
	mu            sync.Mutex
	registered    map[string]string
	requests      []recordedRequest
	putStatus     int
	getStatus     int
	storeOverride string
	emptyBody     bool
}

func newFakeAutomaticWebhookProvider() *fakeAutomaticWebhookProvider {
	return &fakeAutomaticWebhookProvider{registered: map[string]string{}}
}

func (f *fakeAutomaticWebhookProvider) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, recordedRequest{
			method:     r.Method,
			path:       r.URL.Path,
			rawQuery:   r.URL.RawQuery,
			body:       string(body),
			skipHeader: r.Header.Get("x-skip-mtls-checking"),
		})
		if r.URL.Path != "/v2/webhookrec" && r.URL.Path != "/v2/webhookcobr" {
			t.Errorf("unexpected provider path %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
			return
		}
		switch r.Method {
		case http.MethodPut:
			if f.putStatus != 0 {
				w.WriteHeader(f.putStatus)
				return
			}
			var in map[string]string
			if err := json.Unmarshal(body, &in); err != nil {
				t.Errorf("PUT body is not JSON: %s", body)
			}
			stored := in["webhookUrl"]
			if f.storeOverride != "" {
				stored = f.storeOverride
			}
			f.registered[r.URL.Path] = stored
			_ = json.NewEncoder(w).Encode(map[string]string{"webhookUrl": stored})
		case http.MethodGet:
			if f.getStatus != 0 {
				w.WriteHeader(f.getStatus)
				return
			}
			current, ok := f.registered[r.URL.Path]
			if !ok {
				if f.emptyBody {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"nome":"webhook_nao_encontrado"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"webhookUrl": current, "criacao": "2026-09-24T12:00:00.000Z"})
		default:
			t.Errorf("unexpected method %s on %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (f *fakeAutomaticWebhookProvider) calls() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func (f *fakeAutomaticWebhookProvider) count(method string) int {
	n := 0
	for _, r := range f.calls() {
		if r.method == method {
			n++
		}
	}
	return n
}

func newAutomaticWebhookClient(t *testing.T, fake *fakeAutomaticWebhookProvider) *efiapi.EfiClient {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	c, err := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	if err != nil {
		t.Fatalf("NewEfiClient: %v", err)
	}
	return c
}

func assertOnlyAutomaticWebhookPaths(t *testing.T, fake *fakeAutomaticWebhookProvider) {
	t.Helper()
	for _, r := range fake.calls() {
		if strings.HasPrefix(r.path, "/v2/webhook/") || strings.HasPrefix(r.path, "/v3/") || r.path == "/v2/webhook" {
			t.Fatalf("automatic webhook capability touched the Pix key webhook path %s %s", r.method, r.path)
		}
		if r.skipHeader != "" {
			t.Fatalf("automatic webhook capability sent x-skip-mtls-checking=%q on %s %s", r.skipHeader, r.method, r.path)
		}
		if r.rawQuery != "" {
			t.Fatalf("automatic webhook capability sent a query %q on %s %s", r.rawQuery, r.method, r.path)
		}
		if r.method == http.MethodDelete {
			t.Fatalf("automatic webhook capability issued DELETE %s", r.path)
		}
	}
}

func TestEnsureAutomaticWebhook_RegistersEachKindOnItsOwnEndpoint(t *testing.T) {
	for kind, path := range map[string]string{"rec": "/v2/webhookrec", "cobr": "/v2/webhookcobr"} {
		t.Run(kind, func(t *testing.T) {
			fake := newFakeAutomaticWebhookProvider()
			c := newAutomaticWebhookClient(t, fake)

			got, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{
				"kind":        kind,
				"webhook_url": canonicalAutomaticWebhookURL,
			})
			if err != nil {
				t.Fatalf("EnsureAutomaticWebhook: %v", err)
			}
			calls := fake.calls()
			wantOrder := []string{http.MethodGet, http.MethodPut, http.MethodGet}
			if len(calls) != len(wantOrder) {
				t.Fatalf("provider calls = %+v, want GET, PUT, GET", calls)
			}
			for i, method := range wantOrder {
				if calls[i].method != method || calls[i].path != path {
					t.Fatalf("call %d = %s %s, want %s %s", i, calls[i].method, calls[i].path, method, path)
				}
			}
			if calls[1].body != `{"webhookUrl":"`+canonicalAutomaticWebhookURL+`"}` {
				t.Fatalf("PUT body = %s", calls[1].body)
			}
			assertOnlyAutomaticWebhookPaths(t, fake)
			if got["ensured"] != true || got["changed"] != true || got["registered"] != true {
				t.Fatalf("output = %v", got)
			}
			if got["webhook_url"] != canonicalAutomaticWebhookURL || got["kind"] != kind || got["endpoint"] != path {
				t.Fatalf("output = %v", got)
			}
			if got["resource_id"] != strings.TrimPrefix(path, "/v2/") {
				t.Fatalf("resource_id = %v", got["resource_id"])
			}
		})
	}
}

func TestEnsureAutomaticWebhook_AdoptsAMatchingRegistrationWithoutMutation(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.registered["/v2/webhookrec"] = canonicalAutomaticWebhookURL
	c := newAutomaticWebhookClient(t, fake)

	got, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{"kind": "rec", "webhook_url": canonicalAutomaticWebhookURL})
	if err != nil {
		t.Fatalf("EnsureAutomaticWebhook: %v", err)
	}
	if fake.count(http.MethodPut) != 0 {
		t.Fatalf("matching registration was mutated: %+v", fake.calls())
	}
	if got["changed"] != false || got["ensured"] != true {
		t.Fatalf("output = %v", got)
	}
}

func TestEnsureAutomaticWebhook_RepointsADifferentRegistrationOnce(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.registered["/v2/webhookcobr"] = "https://old.example.test/efi"
	c := newAutomaticWebhookClient(t, fake)

	got, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{"kind": "cobr", "webhook_url": canonicalAutomaticWebhookURL})
	if err != nil {
		t.Fatalf("EnsureAutomaticWebhook: %v", err)
	}
	if fake.count(http.MethodPut) != 1 || got["changed"] != true {
		t.Fatalf("want exactly one PUT, calls=%+v output=%v", fake.calls(), got)
	}
}

func TestEnsureAutomaticWebhook_FailsWhenReadbackDiffersAndNeverRetries(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.storeOverride = canonicalAutomaticWebhookURL + "/"
	c := newAutomaticWebhookClient(t, fake)

	_, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{"kind": "rec", "webhook_url": canonicalAutomaticWebhookURL})
	if err == nil || !strings.Contains(err.Error(), "readback does not equal webhook_url") {
		t.Fatalf("expected readback mismatch, got %v", err)
	}
	if fake.count(http.MethodPut) != 1 {
		t.Fatalf("mismatch must not trigger another PUT: %+v", fake.calls())
	}
	if strings.Contains(err.Error(), "webhook-pix.dakasa.me") {
		t.Fatalf("error echoed a URL: %v", err)
	}
}

func TestEnsureAutomaticWebhook_ProviderRejectionIsReturnedWithoutRetry(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.putStatus = http.StatusBadRequest
	c := newAutomaticWebhookClient(t, fake)

	_, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{"kind": "cobr", "webhook_url": canonicalAutomaticWebhookURL})
	if err == nil || !strings.Contains(err.Error(), "register cobr webhook") {
		t.Fatalf("expected registration error, got %v", err)
	}
	if fake.count(http.MethodPut) != 1 || fake.count(http.MethodGet) != 1 {
		t.Fatalf("want one GET and one PUT, got %+v", fake.calls())
	}
}

func TestEnsureAutomaticWebhook_PreflightReadFailureStopsBeforeMutation(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.getStatus = http.StatusForbidden
	c := newAutomaticWebhookClient(t, fake)

	_, err := EnsureAutomaticWebhook(context.Background(), c, map[string]any{"kind": "rec", "webhook_url": canonicalAutomaticWebhookURL})
	if err == nil || !strings.Contains(err.Error(), "before mutation") {
		t.Fatalf("expected preflight read error, got %v", err)
	}
	if fake.count(http.MethodPut) != 0 {
		t.Fatalf("a failed preflight read must not mutate: %+v", fake.calls())
	}
}

func TestEnsureAutomaticWebhook_RefusesMTLSBypassAndPixKeyBeforeAnyProviderCall(t *testing.T) {
	for _, extra := range []map[string]any{
		{"skip_mtls_validation": true},
		{"skip_mtls_validation": false},
		{"chave": "pix@dakasa.me"},
	} {
		fake := newFakeAutomaticWebhookProvider()
		c := newAutomaticWebhookClient(t, fake)
		in := map[string]any{"kind": "rec", "webhook_url": canonicalAutomaticWebhookURL}
		for k, v := range extra {
			in[k] = v
		}
		if _, err := EnsureAutomaticWebhook(context.Background(), c, in); err == nil {
			t.Fatalf("input %v was accepted", extra)
		}
		if _, err := ObserveAutomaticWebhooks(context.Background(), c, in); err == nil {
			t.Fatalf("observe input %v was accepted", extra)
		}
		if len(fake.calls()) != 0 {
			t.Fatalf("refused input %v still reached the provider: %+v", extra, fake.calls())
		}
	}
}

func TestEnsureAutomaticWebhook_ValidatesKindAndURLWithoutEchoingThem(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	c := newAutomaticWebhookClient(t, fake)
	for _, in := range []map[string]any{
		{"webhook_url": canonicalAutomaticWebhookURL},
		{"kind": "pix", "webhook_url": canonicalAutomaticWebhookURL},
		{"kind": 1, "webhook_url": canonicalAutomaticWebhookURL},
		{"kind": "rec"},
		{"kind": "rec", "webhook_url": "http://webhook-pix.dakasa.me/payment/webhook/efi"},
		{"kind": "rec", "webhook_url": "HTTPS://webhook-pix.dakasa.me/payment/webhook/efi"},
		{"kind": "rec", "webhook_url": "https://user:secret@webhook-pix.dakasa.me/payment/webhook/efi"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi?hmac=secret"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi?"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi#secret"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi/"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi/rec"},
		{"kind": "cobr", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi/cobr"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/efi/pix"},
		{"kind": "rec", "webhook_url": "https://webhook-pix.dakasa.me/payment/webhook/%65fi"},
	} {
		_, err := EnsureAutomaticWebhook(context.Background(), c, in)
		if err == nil {
			t.Fatalf("input %v was accepted", in)
		}
		for _, leaked := range []string{"secret", "webhook-pix.dakasa.me"} {
			if strings.Contains(err.Error(), leaked) {
				t.Fatalf("validation error for %v echoed %q: %v", in, leaked, err)
			}
		}
	}
	if len(fake.calls()) != 0 {
		t.Fatalf("invalid input reached the provider: %+v", fake.calls())
	}
}

func TestObserveAutomaticWebhooks_ReportsRegistrationAndAbsence(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.registered["/v2/webhookrec"] = canonicalAutomaticWebhookURL
	c := newAutomaticWebhookClient(t, fake)

	rec, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec"})
	if err != nil {
		t.Fatalf("observe rec: %v", err)
	}
	if rec["registered"] != true || rec["webhook_url"] != canonicalAutomaticWebhookURL || rec["created_at"] != "2026-09-24T12:00:00.000Z" || rec["endpoint"] != "/v2/webhookrec" {
		t.Fatalf("rec = %v", rec)
	}
	if _, gated := rec["matches_expected"]; gated {
		t.Fatalf("ungated observe reported a gate: %v", rec)
	}

	cobr, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "cobr"})
	if err != nil {
		t.Fatalf("observe absent cobr: %v", err)
	}
	if cobr["registered"] != false {
		t.Fatalf("cobr = %v", cobr)
	}
	if _, present := cobr["webhook_url"]; present {
		t.Fatalf("absent registration reported a URL: %v", cobr)
	}
	if fake.count(http.MethodPut) != 0 {
		t.Fatalf("observe mutated: %+v", fake.calls())
	}
	assertOnlyAutomaticWebhookPaths(t, fake)
}

func TestObserveAutomaticWebhooks_TreatsAnEmptyProviderBodyAsAbsent(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.emptyBody = true
	c := newAutomaticWebhookClient(t, fake)

	got, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec"})
	if err != nil || got["registered"] != false {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestObserveAutomaticWebhooks_ExpectedURLIsAnExactReadbackGate(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.registered["/v2/webhookrec"] = canonicalAutomaticWebhookURL
	fake.registered["/v2/webhookcobr"] = canonicalAutomaticWebhookURL + "/other"
	c := newAutomaticWebhookClient(t, fake)

	got, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec", "expected_webhook_url": canonicalAutomaticWebhookURL})
	if err != nil || got["matches_expected"] != true {
		t.Fatalf("matching gate: got %v err %v", got, err)
	}

	_, err = ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "cobr", "expected_webhook_url": canonicalAutomaticWebhookURL})
	if err == nil || !strings.Contains(err.Error(), "readback does not equal expected_webhook_url") {
		t.Fatalf("mismatch gate: %v", err)
	}

	delete(fake.registered, "/v2/webhookcobr")
	_, err = ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "cobr", "expected_webhook_url": canonicalAutomaticWebhookURL})
	if err == nil || !strings.Contains(err.Error(), "registered=false") {
		t.Fatalf("absent gate: %v", err)
	}

	for _, bad := range []any{"", 7, "https://webhook-pix.dakasa.me/payment/webhook/efi/rec"} {
		if _, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec", "expected_webhook_url": bad}); err == nil {
			t.Fatalf("expected_webhook_url %v was accepted", bad)
		}
	}
	assertOnlyAutomaticWebhookPaths(t, fake)
}

func TestObserveAutomaticWebhooks_ProviderFailureIsAnError(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.getStatus = http.StatusServiceUnavailable
	c := newAutomaticWebhookClient(t, fake)

	if _, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec"}); err == nil {
		t.Fatal("a 503 was reported as a successful observation")
	}
}

func TestObserveAutomaticWebhooks_RedactsQueryValuesRegisteredElsewhere(t *testing.T) {
	fake := newFakeAutomaticWebhookProvider()
	fake.registered["/v2/webhookrec"] = "https://legacy.example.test/efi?hmac=must-not-escape"
	c := newAutomaticWebhookClient(t, fake)

	got, err := ObserveAutomaticWebhooks(context.Background(), c, map[string]any{"kind": "rec"})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if strings.Contains(got["webhook_url"].(string), "must-not-escape") {
		t.Fatalf("observed URL leaked a query value: %v", got["webhook_url"])
	}
}

func TestAutomaticWebhookResourceID(t *testing.T) {
	if AutomaticWebhookResourceID("rec") != "webhookrec" || AutomaticWebhookResourceID("cobr") != "webhookcobr" || AutomaticWebhookResourceID("pix") != "" {
		t.Fatal("resource identity changed")
	}
}
