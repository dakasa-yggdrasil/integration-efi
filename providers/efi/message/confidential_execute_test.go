package message

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/crypto/nacl/box"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/confidential"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

const (
	confWireTarget    = "655fdf27-655d-4279-89fc-9fb705432d35"
	confWireOtherEVP  = "1ed106ab-5f9c-49a5-9f9d-fb0921b82e11"
	confWireOtherCPF  = "52890437106"
	confWireOtherMail = "other-private-owner@example.test"
	confWireNested    = "private-provider-nested-value"
	confWireQuery     = "private-provider-query-value"
	confWireBase      = "https://receiver.example.test/negotiation"
	confWireStart     = "1970-01-01T00:00:00.000Z"
	confWireEnd       = "2099-12-31T23:59:59.000Z"
	confWireInstance  = "confidential-selected-fixture-instance"
)

type confidentialWireHarness struct {
	t                           *testing.T
	mu                          sync.Mutex
	clock                       time.Time
	baseTime                    time.Time
	receiver                    *confidential.Receiver
	resultPublic, resultPrivate *[32]byte
	comparison                  [32]byte
	providerURL                 string
	provider                    func(http.ResponseWriter, *http.Request)
	eventStatus                 int
	eventBody                   string
	requests                    []string
	oauth                       int
	puts                        int
	events                      [][]byte
	logs                        bytes.Buffer
	a                           *sdkadapter.Adapter
}

// This fixture runs ExecuteHandler, WireReconcilers, the real SDK dispatcher,
// per-instance OAuth and the real provider client. Only external HTTP endpoints
// are replaced. No dispatcher, client constructor, reconciler or emitter is mocked.
func newConfidentialWireHarness(t *testing.T) *confidentialWireHarness {
	t.Helper()
	h := &confidentialWireHarness{t: t, clock: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), eventStatus: http.StatusAccepted}
	h.baseTime = h.clock
	var err error
	h.receiver, err = confidential.NewReceiver(adapter.AdapterVersion)
	if err != nil {
		t.Fatal(err)
	}
	h.resultPublic, h.resultPrivate, err = box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(rand.Reader, h.comparison[:]); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			key, secret, ok := r.BasicAuth()
			if !ok || key != "selected-client-key" || secret != "selected-client-secret" {
				t.Error("OAuth did not use selected instance credentials")
			}
			h.mu.Lock()
			h.oauth++
			h.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "selected-provider-token"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer selected-provider-token" || r.Header.Get("x-skip-mtls-checking") != "" {
			t.Error("provider authentication or mTLS skip boundary changed")
		}
		h.mu.Lock()
		h.requests = append(h.requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			h.puts++
		}
		h.mu.Unlock()
		if r.Method == http.MethodGet && r.URL.Path == "/v2/webhook" {
			q := r.URL.Query()
			if q.Get("paginacao.paginaAtual") != "0" || q.Get("paginacao.itensPorPagina") != "1000" {
				t.Error("provider LIST was not page0/size1000")
			}
		}
		if h.provider == nil {
			t.Error("unexpected provider request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.provider(w, r)
	}))
	t.Cleanup(provider.Close)
	h.providerURL = provider.URL
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/events" || r.Header.Get("Authorization") != "Bearer fixture-event-token" {
			t.Error("unexpected event HTTP contract")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		h.mu.Lock()
		h.events = append(h.events, raw)
		status, body := h.eventStatus, h.eventBody
		h.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(core.Close)
	t.Setenv("YGGDRASIL_CORE_URL", core.URL)
	t.Setenv("YGGDRASIL_RUN_TOKEN", "fixture-event-token")
	t.Setenv("EFI_API_CLIENT_KEY_ID", "wrong-environment-client")
	t.Setenv("EFI_API_CLIENT_SECRET", "wrong-environment-secret")
	t.Setenv("EFI_BASE_URL", "https://environment-must-not-be-used.invalid")
	h.a = sdkadapter.New(sdkadapter.Config{Provider: adapter.Provider, IntegrationType: adapter.IntegrationType, Version: adapter.AdapterVersion})
	adapter.WireReconcilers(h.a, "wrong-fallback-instance")
	return h
}

func (h *confidentialWireHarness) now() time.Time        { h.mu.Lock(); defer h.mu.Unlock(); return h.clock }
func (h *confidentialWireHarness) setTime(now time.Time) { h.mu.Lock(); h.clock = now; h.mu.Unlock() }
func (h *confidentialWireHarness) counts() (int, int, int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.requests), h.oauth, h.puts, len(h.events)
}

func (h *confidentialWireHarness) request(purpose string) confidential.Request {
	op := adapter.OperationObserveWebhookSubscriptions
	before := ""
	if purpose == "ensure" {
		op = adapter.OperationEnsureWebhookSubscription
		before = "absent"
	}
	metadata := h.receiver.Metadata()
	public, err := confidential.PublicKey(metadata.PublicKey)
	if err != nil {
		h.t.Fatal(err)
	}
	req := confidential.Request{Intent: confidential.Intent{
		SchemaVersion: 1,
		Binding: confidential.Binding{
			SourceRoot: strings.Repeat("a", 40), ReleaseSHA256: strings.Repeat("b", 64),
			InvocationID: "b86638b2-de7c-4fa1-9095-e0d4bf2c34c1", Mode: "fixture-webhook-" + purpose,
			WorkflowID: "fixture-confidential-workflow", WorkflowVersion: 1, StepID: "fixture-" + purpose,
			Operation: op, Purpose: purpose, InstanceName: confWireInstance,
			ReceiverKeyID: metadata.KeyID, ReceiverPublicKeySHA256: confidential.Fingerprint(public[:]),
			ResultPublicKeySHA256: confidential.Fingerprint(h.resultPublic[:]), ComparisonSecretSHA256: confidential.Fingerprint(h.comparison[:]),
			AuditResourceID: "077c00ec-4500-4170-9665-a93a549ff210",
			IssuedAt:        h.baseTime.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: h.baseTime.Add(4 * time.Minute).Format(time.RFC3339Nano),
		},
		Parameters:      confidential.Parameters{Key: confWireTarget, CanonicalBase: confWireBase, Start: confWireStart, End: confWireEnd, Page: 0, PageSize: 1000, BeforeState: before},
		ResultPublicKey: base64.StdEncoding.EncodeToString(h.resultPublic[:]), ComparisonSecret: base64.StdEncoding.EncodeToString(h.comparison[:]),
	}}
	return req
}

func (h *confidentialWireHarness) input(req confidential.Request) map[string]any {
	h.t.Helper()
	var err error
	req.IntentSHA256, err = confidential.IntentHash(req.Intent)
	if err != nil {
		h.t.Fatal(err)
	}
	sealed, err := confidential.SealInput(req, h.receiver.Metadata(), rand.Reader)
	if err != nil {
		h.t.Fatal(err)
	}
	var input map[string]any
	if json.Unmarshal(confWireJSON(h.t, sealed), &input) != nil {
		h.t.Fatal("cannot encode actual sealed Input type")
	}
	return input
}

func (h *confidentialWireHarness) envelope(operation string, input map[string]any) map[string]any {
	return map[string]any{"operation": operation, "capability": operation, "input": input, "integration": map[string]any{
		"instance":      map[string]any{"name": confWireInstance},
		"instance_spec": map[string]any{"credentials": map[string]any{"efi_client_key_id": "selected-client-key", "efi_client_secret": "selected-client-secret"}, "config": map[string]any{"base_url": h.providerURL, "mtls_enabled": false}},
	}}
}

type confidentialWireReply struct {
	response rpcResponse
	output   confidential.Output
	raw      []byte
	legacy   map[string]any
}

func (h *confidentialWireHarness) callRaw(raw []byte, receiver *confidential.Receiver) confidentialWireReply {
	h.t.Helper()
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&h.logs), zap.DebugLevel))
	handler := ExecuteHandler(logger, h.a, WithConfidentialReceiver(receiver), func(o *executeOptions) { o.now = h.now })
	body, contentType, err := handler(context.Background(), rpc.Delivery{Body: raw, ContentType: "application/json"})
	if err != nil || contentType != "application/json" {
		h.t.Fatalf("actual handler RPC contract: %v %q", err, contentType)
	}
	var reply confidentialWireReply
	reply.raw = body
	if json.Unmarshal(body, &reply.response) != nil {
		h.t.Fatal("invalid actual handler RPC JSON")
	}
	if reply.response.OK {
		var data struct {
			Operation string          `json:"operation"`
			Output    json.RawMessage `json:"output"`
			Metadata  map[string]any  `json:"metadata"`
		}
		if json.Unmarshal(confWireJSON(h.t, reply.response.Data), &data) != nil || data.Metadata["provider"] != adapter.Provider {
			h.t.Fatal("actual adapter response identity changed")
		}
		if json.Unmarshal(data.Output, &reply.output) != nil || json.Unmarshal(data.Output, &reply.legacy) != nil {
			h.t.Fatal("invalid output object")
		}
	}
	return reply
}
func (h *confidentialWireHarness) call(req confidential.Request) confidentialWireReply {
	return h.callWithMetadata(req, nil)
}
func (h *confidentialWireHarness) callWithMetadata(req confidential.Request, metadata map[string]any) confidentialWireReply {
	input := h.input(req)
	h.assertPrivateAbsent(confWireJSON(h.t, input))
	envelope := h.envelope(req.Binding.Operation, input)
	if metadata != nil {
		envelope["metadata"] = metadata
	}
	reply := h.callRaw(confWireJSON(h.t, envelope), h.receiver)
	if reply.response.OK {
		confWirePublicShape(h.t, reply.legacy)
	}
	return reply
}
func (h *confidentialWireHarness) decrypt(req confidential.Request, reply confidentialWireReply) confidential.PlainOutput {
	h.t.Helper()
	if !reply.response.OK {
		h.t.Fatalf("confidential request failed: %+v", reply.response.Error)
	}
	plain, err := confidential.OpenOutput(reply.output, h.resultPublic, h.resultPrivate)
	if err != nil {
		h.t.Fatalf("actual handler output cannot decrypt: %v", err)
	}
	wantHash, err := confidential.IntentHash(req.Intent)
	if err != nil || plain.IntentSHA256 != wantHash || !reflect.DeepEqual(plain.Binding, req.Binding) {
		h.t.Fatal("sealed response lost immutable request binding")
	}
	return plain
}
func (h *confidentialWireHarness) assertPrivateAbsent(raw []byte) {
	h.t.Helper()
	for _, secret := range []string{confWireTarget, confWireOtherCPF, confWireOtherMail, confWireOtherEVP, confWireNested, confWireQuery, confWireBase, "https://other.example.test", "selected-client-secret", "selected-provider-token", base64.StdEncoding.EncodeToString(h.resultPrivate[:]), base64.StdEncoding.EncodeToString(h.comparison[:])} {
		if bytes.Contains(raw, []byte(secret)) {
			h.t.Fatal("private value crossed a public wire/event/diagnostic boundary")
		}
	}
}
func (h *confidentialWireHarness) assertSafeEvent(reply confidentialWireReply, req confidential.Request, index int) {
	h.t.Helper()
	h.mu.Lock()
	raw := append([]byte(nil), h.events[index]...)
	h.mu.Unlock()
	h.assertPrivateAbsent(raw)
	var event struct {
		Type        string              `json:"event_type"`
		ResourceID  string              `json:"resource_id"`
		Instance    string              `json:"instance_id"`
		Idempotency string              `json:"idempotency"`
		Observed    confidential.Output `json:"observed"`
	}
	intentHash, err := confidential.IntentHash(req.Intent)
	if err != nil {
		h.t.Fatal(err)
	}
	idempotencyHash := sha256.Sum256([]byte("efi.webhook.idempotency.v1\x00" + intentHash))
	wantIdempotency := fmt.Sprintf("efi-confidential-v1:%x", idempotencyHash)
	if json.Unmarshal(raw, &event) != nil || event.Type != "efi.webhook_subscription.ensured" || event.ResourceID != req.Binding.AuditResourceID || event.Instance != confWireInstance || event.Idempotency != wantIdempotency || !reflect.DeepEqual(event.Observed, reply.output) {
		h.t.Fatal("SDK did not emit the actual safe Observed DTO with opaque identity")
	}
}

func confWireJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func confWirePublicShape(t *testing.T, output map[string]any) {
	t.Helper()
	fields := func(value map[string]any, allowed ...string) {
		t.Helper()
		for name := range value {
			found := false
			for _, candidate := range allowed {
				found = found || name == candidate
			}
			if !found {
				t.Fatal("public projection contains an unreviewed field")
			}
		}
	}
	fields(output, "id", "privacy", "proof")
	privacy, ok := output["privacy"].(map[string]any)
	if !ok || len(privacy) != 3 {
		t.Fatal("public privacy envelope is not closed")
	}
	fields(privacy, "schemaVersion", "algorithm", "ciphertext")
	proof, ok := output["proof"].(map[string]any)
	if !ok {
		t.Fatal("public proof is missing")
	}
	fields(proof, "schemaVersion", "kind", "outcome", "provider", "cursor", "parametros", "page_provenance", "rows", "key_tag", "url_tag", "creation_valid", "ensured", "noop", "endpoint")
	if raw, exists := proof["rows"]; exists {
		rows, ok := raw.([]any)
		if !ok {
			t.Fatal("public row vector changed type")
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok || len(row) != 3 {
				t.Fatal("public row is not a closed tag projection")
			}
			fields(row, "key_tag", "url_tag", "creation_valid")
		}
	}
}
func confWireRecord(key, address string) map[string]any {
	return map[string]any{"chave": key, "webhookUrl": address, "criacao": "2026-10-30T12:00:00.000Z"}
}
func confWireOthers() []any {
	return []any{confWireRecord(confWireOtherCPF, "https://other.example.test/cpf?token="+confWireQuery), confWireRecord(confWireOtherMail, "https://other.example.test/email"), confWireRecord(confWireOtherEVP, "https://other.example.test/evp")}
}
func confWirePage(rows []any) map[string]any {
	return map[string]any{"parametros": map[string]any{"inicio": confWireStart, "fim": confWireEnd, "paginacao": map[string]any{"paginaAtual": 0, "itensPorPagina": 1000, "quantidadeDePaginas": 1, "quantidadeTotalDeItens": len(rows)}}, "webhooks": rows, "unknown": map[string]any{"nested": map[string]any{"value": confWireNested, "key": confWireOtherMail}}}
}
func confWireWrite(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func TestConfidentialExecuteHandlerPreservesCompleteLegacyListWithoutPublicPII(t *testing.T) {
	h := newConfidentialWireHarness(t)
	rows := append(confWireOthers(), confWireRecord(confWireTarget, confWireBase))
	rows[0].(map[string]any)["unknown"] = map[string]any{"private": confWireNested, "query_original": confWireQuery}
	page := confWirePage(rows)
	h.provider = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/webhook" || r.URL.Query().Get("inicio") != confWireStart || r.URL.Query().Get("fim") != confWireEnd {
			t.Error("unexpected closed LIST request")
		}
		confWireWrite(w, page)
	}
	req := h.request("list")
	reply := h.call(req)
	plain := h.decrypt(req, reply)
	h.assertPrivateAbsent(reply.raw)
	h.assertPrivateAbsent(h.logs.Bytes())
	// Obtain the comparison DTO through the actual legacy handler, rather than
	// copying its aliases or its provider-field merge in this test.
	legacy := h.callRaw(confWireJSON(t, h.envelope(adapter.OperationObserveWebhookSubscriptions, map[string]any{"inicio": confWireStart, "fim": confWireEnd, "page": 0, "page_size": 1000})), h.receiver)
	var decrypted map[string]any
	if json.Unmarshal(plain.LegacyOutput, &decrypted) != nil || !legacy.response.OK || !reflect.DeepEqual(decrypted, legacy.legacy) {
		t.Fatal("encryption lost fields or aliases from the actual legacy DTO")
	}
	if !bytes.Contains(plain.LegacyOutput, []byte(confWireNested)) || !bytes.Contains(plain.LegacyOutput, []byte(confWireQuery)) {
		t.Fatal("unknown private fields were erased instead of encrypted")
	}
	proof := reply.output.Proof
	if proof.Kind != "list" || proof.Rows == nil || len(*proof.Rows) != len(rows) || proof.PageProvenance == nil || !proof.PageProvenance.Validated || !proof.PageProvenance.Complete {
		t.Fatal("complete page lost its closed projection")
	}
	for i, record := range decrypted["webhooks"].([]any) {
		row := record.(map[string]any)
		keyTag, _ := confidential.Tag(&h.comparison, confidential.KeyTagDomain, req.Binding.InvocationID, row["chave"].(string))
		urlTag, _ := confidential.Tag(&h.comparison, confidential.URLTagDomain, req.Binding.InvocationID, row["webhookUrl"].(string))
		if (*proof.Rows)[i].KeyTag != keyTag || (*proof.Rows)[i].URLTag != urlTag || !(*proof.Rows)[i].CreationValid {
			t.Fatal("projection changed row order, exact emitted strings or domains")
		}
	}
	requests, oauth, puts, events := h.counts()
	if requests != 2 || oauth != 2 || puts != 0 || events != 0 {
		t.Fatal("read-only confidential/legacy comparison caused extra calls or a mutation event")
	}
}

func TestConfidentialExecuteHandlerRetainsFrozenActualHandlerDTOs(t *testing.T) {
	for _, purpose := range []string{"list", "single"} {
		t.Run(purpose, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			name := "webhook-observation-list.v1.json"
			if purpose == "single" {
				name = "webhook-observation-single.v1.json"
			}
			fixture := webhookWireFixture(t, name)
			req := h.request(purpose)
			// This unchanged generic 2.5.6 actual-handler fixture retains its own
			// echoed dates. It is not evidence that a provider accepted closed2099.
			var provider map[string]any
			if purpose == "list" {
				provider = map[string]any{"parametros": fixture["parametros"], "webhooks": fixture["webhooks"]}
				p := fixture["parametros"].(map[string]any)
				req.Parameters.Start = p["inicio"].(string)
				req.Parameters.End = p["fim"].(string)
			} else {
				provider = map[string]any{"chave": fixture["chave"], "webhookUrl": fixture["webhookUrl"], "criacao": fixture["criacao"]}
				req.Parameters.Key = fixture["chave"].(string)
				req.Parameters.CanonicalBase = fixture["webhookUrl"].(string)
			}
			h.provider = func(w http.ResponseWriter, r *http.Request) { confWireWrite(w, provider) }
			reply := h.call(req)
			plain := h.decrypt(req, reply)
			var actual map[string]any
			if json.Unmarshal(plain.LegacyOutput, &actual) != nil || !reflect.DeepEqual(actual, fixture) {
				t.Fatal("sealed actual handler changed an immutable legacy fixture DTO")
			}
			if bytes.Contains(reply.raw, []byte("11111111-1111-4111-8111-111111111111")) {
				t.Fatal("fixture key escaped encryption")
			}
		})
	}
}

func TestConfidentialEnsureUsesOwnCompleteListAndSDKEvent(t *testing.T) {
	for _, initialCanonical := range []bool{false, true} {
		t.Run(fmt.Sprint(initialCanonical), func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			registered := initialCanonical
			h.provider = func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v2/webhook":
					rows := confWireOthers()
					if registered {
						rows = append(rows, confWireRecord(confWireTarget, confWireBase))
					}
					confWireWrite(w, confWirePage(rows))
				case r.Method == http.MethodPut && r.URL.Path == "/v2/webhook/"+confWireTarget:
					var body map[string]string
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["webhookUrl"] != confWireBase {
						t.Error("PUT changed canonical receiver")
					}
					registered = true
					w.WriteHeader(http.StatusNoContent)
				case r.Method == http.MethodGet && r.URL.Path == "/v2/webhook/"+confWireTarget:
					if !registered {
						t.Error("confirmation ran before registration")
					}
					record := confWireRecord(confWireTarget, confWireBase)
					record["unknown"] = map[string]any{"private": confWireNested}
					confWireWrite(w, record)
				default:
					t.Error("unexpected ensure provider operation")
					w.WriteHeader(http.StatusInternalServerError)
				}
			}
			req := h.request("ensure")
			if initialCanonical {
				req.Parameters.BeforeState = "canonical"
			}
			reply := h.call(req)
			plain := h.decrypt(req, reply)
			if reply.output.Proof.Ensured == nil || !*reply.output.Proof.Ensured || reply.output.Proof.Noop == nil || *reply.output.Proof.Noop || reply.output.Proof.Endpoint != "v2" || reply.output.Proof.Outcome != "confirmed" {
				t.Fatal("confirmed ensure omitted its mandatory actual PUT/mTLS gate")
			}
			if len(plain.GuardObservation) == 0 || len(plain.GuardConfirmation) == 0 || !bytes.Contains(plain.GuardObservation, []byte(confWireOtherCPF)) || !bytes.Contains(plain.GuardConfirmation, []byte(confWireNested)) {
				t.Fatal("complete internal guard/confirmation DTOs were not sealed")
			}
			var legacy map[string]any
			if json.Unmarshal(plain.LegacyOutput, &legacy) != nil || legacy["chave"] != confWireTarget || legacy["endpoint"] != "v2" || legacy["ensured"] != true || legacy["noop"] != false {
				t.Fatal("sealed ensure legacy output does not describe the actual handler outcome")
			}
			calls, oauth, puts, events := h.counts()
			if calls != 3 || oauth != 3 || puts != 1 || events != 1 {
				t.Fatalf("unexpected ensure calls: provider=%d OAuth=%d PUT=%d events=%d", calls, oauth, puts, events)
			}
			h.mu.Lock()
			actualCalls := append([]string(nil), h.requests...)
			h.mu.Unlock()
			want := []string{"GET /v2/webhook", "PUT /v2/webhook/" + confWireTarget, "GET /v2/webhook/" + confWireTarget}
			if !reflect.DeepEqual(actualCalls, want) {
				t.Fatal("own LIST, mutation and exact GET ordering changed")
			}
			h.assertSafeEvent(reply, req, 0)
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
			// A second logical delivery must retain the actual setter/mTLS gate.
			// It may repeat PUT within the authorization window; no physical
			// exactly-once or Root replay claim is made by this handler test.
			repeated := h.call(req)
			h.decrypt(req, repeated)
			if repeated.output.Proof.Noop == nil || *repeated.output.Proof.Noop || repeated.output.Privacy.Ciphertext == reply.output.Privacy.Ciphertext {
				t.Fatal("repeat bypassed the setter or reused its sealed result")
			}
			calls, oauth, puts, events = h.counts()
			if calls != 6 || oauth != 6 || puts != 2 || events != 2 {
				t.Fatal("second handler delivery did not retain LIST/actual PUT/GET/event")
			}
			h.assertSafeEvent(repeated, req, 1)
		})
	}
}

func TestConfidentialEnsureRefusesUnprovenOwnListBeforePUT(t *testing.T) {
	for _, name := range []string{"foreign", "missing-pagination", "wrong-range", "duplicate-key", "noncanonical-uuid", "invalid-query", "missing-created", "paged", "malformed-json", "duplicate-json", "400", "403", "404", "expired-at-entry", "expired-after-list"} {
		t.Run(name, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			req := h.request("ensure")
			rows := confWireOthers()
			page := confWirePage(rows)
			switch name {
			case "foreign":
				page["webhooks"] = append(rows, confWireRecord(confWireTarget, "https://foreign.example.test/receiver"))
				page["parametros"].(map[string]any)["paginacao"].(map[string]any)["quantidadeTotalDeItens"] = 4
			case "missing-pagination":
				delete(page["parametros"].(map[string]any), "paginacao")
			case "wrong-range":
				page["parametros"].(map[string]any)["inicio"] = "2020-11-16T00:00:00.000Z"
			case "duplicate-key":
				rows[1].(map[string]any)["chave"] = confWireOtherCPF
			case "noncanonical-uuid":
				rows[2].(map[string]any)["chave"] = strings.ToUpper(confWireOtherEVP)
			case "invalid-query":
				rows[0].(map[string]any)["webhookUrl"] = "https://other.example.test/webhook?secret=%ZZ"
			case "missing-created":
				delete(rows[0].(map[string]any), "criacao")
			case "paged":
				many := make([]any, 1000)
				for i := range many {
					many[i] = confWireRecord(fmt.Sprintf("%08x-0000-4000-8000-%012x", i+1, i+1), "https://other.example.test/page")
				}
				page = confWirePage(many)
				p := page["parametros"].(map[string]any)["paginacao"].(map[string]any)
				p["quantidadeDePaginas"], p["quantidadeTotalDeItens"] = 2, 1001
			case "expired-at-entry":
				req.Binding.IssuedAt = h.baseTime.Add(-2 * time.Minute).Format(time.RFC3339Nano)
				req.Binding.ExpiresAt = h.baseTime.Format(time.RFC3339Nano)
			}
			h.provider = func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v2/webhook" {
					t.Error("unproven page reached a non-LIST provider operation")
				}
				switch name {
				case "400", "403", "404":
					var status int
					_, _ = fmt.Sscan(name, &status)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"chave":"`+confWireOtherCPF+`","error":"`+confWireNested+`"}`)
				case "malformed-json":
					_, _ = io.WriteString(w, `{"private":"`+confWireNested+`",`)
				case "duplicate-json":
					raw := confWireJSON(t, page)
					_, _ = w.Write(append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"unknown":{"private":"`+confWireOtherCPF+`"}}`)...))
				default:
					confWireWrite(w, page)
				}
				if name == "expired-after-list" {
					h.setTime(h.baseTime.Add(4 * time.Minute))
				}
			}
			reply := h.call(req)
			if reply.response.OK || reply.response.Error == nil {
				t.Fatal("unproven page authorized ensure")
			}
			calls, oauth, puts, events := h.counts()
			if puts != 0 || events != 0 {
				t.Fatal("unproven own LIST caused mutation or success event")
			}
			if name == "expired-at-entry" && (calls != 0 || oauth != 0) {
				t.Fatal("expired input accessed provider before rejection")
			}
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
		})
	}
}

func TestConfidentialExecuteHandlerRejectsInvalidWireBeforeProviderAccess(t *testing.T) {
	for _, name := range []string{"mixed-raw", "wrong-case", "wrong-receiver", "restarted-receiver", "missing-receiver", "wrong-result-recipient", "wrong-comparison-secret", "wrong-instance", "skip-mtls", "bad-ciphertext", "wrong-operation", "unsupported-operation", "duplicate-input-hidden-confidential", "duplicate-envelope-member", "wrong-case-top-member", "malformed-json"} {
		t.Run(name, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			req := h.request("ensure")
			input := h.input(req)
			envelope := h.envelope(req.Binding.Operation, input)
			receiver := h.receiver
			switch name {
			case "mixed-raw":
				input["chave"] = confWireTarget
			case "wrong-case":
				input["Confidential_Input"] = input["confidential_input"]
				delete(input, "confidential_input")
			case "wrong-receiver", "restarted-receiver":
				var err error
				receiver, err = confidential.NewReceiver(adapter.AdapterVersion)
				if err != nil {
					t.Fatal(err)
				}
			case "missing-receiver":
				receiver = nil
			case "wrong-result-recipient":
				req.ResultPublicKey = h.receiver.Metadata().PublicKey
				envelope["input"] = h.input(req)
			case "wrong-comparison-secret":
				req.ComparisonSecret = base64.StdEncoding.EncodeToString(make([]byte, 32))
				envelope["input"] = h.input(req)
			case "wrong-instance":
				envelope["integration"].(map[string]any)["instance"].(map[string]any)["name"] = "other-instance"
			case "skip-mtls":
				req.Parameters.SkipMTLSValidation = true
				envelope["input"] = h.input(req)
			case "bad-ciphertext":
				input["confidential_input"].(map[string]any)["ciphertext"] = "AAAA"
			case "wrong-operation":
				envelope["operation"], envelope["capability"] = adapter.OperationObserveWebhookSubscriptions, adapter.OperationObserveWebhookSubscriptions
			case "unsupported-operation":
				envelope["operation"], envelope["capability"] = confWireNested, confWireNested
			case "wrong-case-top-member":
				envelope["Input"] = input
				delete(envelope, "input")
			}
			raw := confWireJSON(t, envelope)
			switch name {
			case "duplicate-input-hidden-confidential":
				// A later raw input must not erase the earlier opt-in marker and
				// fall through into a legacy unconditional PUT.
				envelope["input"] = map[string]any{"chave": confWireTarget, "webhook_url": confWireBase}
				raw = confWireJSON(t, envelope)
				prefix := append([]byte(`"input":`), confWireJSON(t, input)...)
				prefix = append(prefix, []byte(`,"input":`)...)
				raw = bytes.Replace(raw, []byte(`"input":`), prefix, 1)
			case "duplicate-envelope-member":
				raw = bytes.Replace(raw, []byte(`"operation":`), []byte(`"operation":"`+adapter.OperationEnsureWebhookSubscription+`","operation":`), 1)
			case "malformed-json":
				raw = append([]byte(nil), raw[:len(raw)-1]...)
			}
			reply := h.callRaw(raw, receiver)
			if reply.response.OK || reply.response.Error == nil {
				t.Fatal("invalid confidential request was accepted")
			}
			calls, oauth, puts, events := h.counts()
			if calls != 0 || oauth != 0 || puts != 0 || events != 0 {
				t.Fatal("invalid confidential envelope reached provider or event I/O")
			}
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
		})
	}
}

func TestConfidentialCanonicalSetterRequiresExactSingleReadback(t *testing.T) {
	for _, name := range []string{"wrong-key", "foreign-url", "missing-key", "unsafe-original-url", "duplicate-json"} {
		t.Run(name, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			req := h.request("ensure")
			req.Parameters.BeforeState = "canonical"
			h.provider = func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.URL.Path == "/v2/webhook" {
					confWireWrite(w, confWirePage([]any{confWireRecord(confWireTarget, confWireBase)}))
					return
				}
				record := confWireRecord(confWireTarget, confWireBase)
				switch name {
				case "wrong-key":
					record["chave"] = confWireOtherEVP
				case "foreign-url":
					record["webhookUrl"] = "https://foreign.example.test/receiver"
				case "missing-key":
					delete(record, "chave")
				case "unsafe-original-url":
					record["webhookUrl"] = confWireBase + "#" + confWireNested
				case "duplicate-json":
					raw := confWireJSON(t, record)
					_, _ = w.Write(append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"chave":"`+confWireTarget+`"}`)...))
					return
				}
				confWireWrite(w, record)
			}
			reply := h.call(req)
			if !reply.response.OK || reply.output.Proof.Outcome != "held" || reply.output.Proof.Ensured == nil || *reply.output.Proof.Ensured || reply.output.Privacy.Ciphertext != "" {
				t.Fatal("canonical setter/readback failure lost its held outcome or falsely confirmed success")
			}
			calls, oauth, puts, events := h.counts()
			if calls != 3 || oauth != 3 || puts != 1 || events != 1 {
				t.Fatal("canonical setter/readback failure lost the actual PUT or safe event attempt")
			}
			if _, err := confidential.OpenOutput(reply.output, h.resultPublic, h.resultPrivate); err == nil {
				t.Fatal("unproven canonical readback became B7 evidence")
			}
			h.assertSafeEvent(reply, req, 0)
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
		})
	}
}

func TestConfidentialEnsurePreservesSafeEventAfterKnownMutationFailure(t *testing.T) {
	for _, initialCanonical := range []bool{false, true} {
		t.Run(fmt.Sprint(initialCanonical), func(t *testing.T) {
			for _, name := range []string{"confirm-400", "confirm-404", "confirm-malformed", "confirm-oversize", "put-oversize", "unknown-put-transport"} {
				t.Run(name, func(t *testing.T) {
					h := newConfidentialWireHarness(t)
					req := h.request("ensure")
					if initialCanonical {
						req.Parameters.BeforeState = "canonical"
					}
					h.provider = func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/v2/webhook" {
							rows := confWireOthers()
							if initialCanonical {
								rows = append(rows, confWireRecord(confWireTarget, confWireBase))
							}
							confWireWrite(w, confWirePage(rows))
							return
						}
						if r.Method == http.MethodPut {
							if name == "unknown-put-transport" {
								connection, _, err := w.(http.Hijacker).Hijack()
								if err != nil {
									t.Error(err)
									return
								}
								_ = connection.Close()
								return
							}
							if name == "put-oversize" {
								_, _ = io.WriteString(w, strings.Repeat("x", efiapi.MaxConfidentialResponseBytes+1))
								return
							}
							w.WriteHeader(http.StatusNoContent)
							return
						}
						switch name {
						case "confirm-400":
							w.WriteHeader(http.StatusBadRequest)
							_, _ = io.WriteString(w, confWireNested)
						case "confirm-404":
							w.WriteHeader(http.StatusNotFound)
							_, _ = io.WriteString(w, confWireOtherCPF)
						case "confirm-malformed":
							_, _ = io.WriteString(w, `{"private":"`+confWireNested+`",`)
						case "confirm-oversize":
							record := confWireRecord(confWireTarget, confWireBase)
							record["unknown"] = strings.Repeat("x", efiapi.MaxConfidentialResponseBytes)
							confWireWrite(w, record)
						default:
							t.Error("unexpected post-PUT confirmation")
						}
					}
					reply := h.call(req)
					_, _, puts, events := h.counts()
					if puts != 1 {
						t.Fatal("post-mutation failure caused missing or repeated PUT")
					}
					if name == "unknown-put-transport" {
						if reply.response.OK || events != 0 {
							t.Fatal("unknown PUT transport acquired a successful event or response")
						}
					} else {
						if !reply.response.OK || reply.output.Proof.Outcome != "held" || reply.output.Proof.Ensured == nil || *reply.output.Proof.Ensured || reply.output.Privacy.Ciphertext != "" || events != 1 {
							t.Fatal("known mutation failure lost its safe held event or falsely confirmed success")
						}
						if _, err := confidential.OpenOutput(reply.output, h.resultPublic, h.resultPrivate); err == nil {
							t.Fatal("held outcome could be consumed as B7 raw evidence")
						}
						h.assertSafeEvent(reply, req, 0)
					}
					h.assertPrivateAbsent(reply.raw)
					h.assertPrivateAbsent(h.logs.Bytes())
				})
			}
		})
	}
}

func TestConfidentialSDKEmitterMasksReflectedErrorBeforeWarning(t *testing.T) {
	h := newConfidentialWireHarness(t)
	req := h.request("ensure")
	req.Parameters.BeforeState = "canonical"
	h.eventStatus = http.StatusForbidden
	h.eventBody = confWireOtherCPF + " " + confWireOtherMail + " " + confWireNested
	h.provider = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v2/webhook" {
			confWireWrite(w, confWirePage([]any{confWireRecord(confWireTarget, confWireBase)}))
		} else {
			confWireWrite(w, confWireRecord(confWireTarget, confWireBase))
		}
	}
	// The pinned SDK's default warning sink uses fmt.Printf, not the supplied
	// handler zap logger. Capture that actual first sink without replacing it.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; _ = reader.Close(); _ = writer.Close() }()
	reply := h.call(req)
	os.Stdout = original
	_ = writer.Close()
	warning, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	h.decrypt(req, reply)
	if !bytes.Contains(warning, []byte("confidential mutation event delivery failed")) {
		t.Fatal("actual SDK warning did not receive the static emitter error")
	}
	h.assertPrivateAbsent(warning)
	h.assertPrivateAbsent(reply.raw)
	h.assertPrivateAbsent(h.logs.Bytes())
	_, _, puts, events := h.counts()
	if puts != 1 || events != 1 {
		t.Fatal("canonical setter or terminal event failure lost the actual PUT or retried")
	}
	h.assertSafeEvent(reply, req, 0)
}

func TestConfidentialCallerMetadataCannotBecomeEventIdentity(t *testing.T) {
	h := newConfidentialWireHarness(t)
	req := h.request("ensure")
	req.Parameters.BeforeState = "canonical"
	h.provider = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v2/webhook" {
			confWireWrite(w, confWirePage([]any{confWireRecord(confWireTarget, confWireBase)}))
		} else {
			confWireWrite(w, confWireRecord(confWireTarget, confWireBase))
		}
	}
	for i, maliciousIdempotency := range []string{confWireTarget, confWireOtherCPF} {
		// These deliberately unsafe caller fields must not reach the SDK
		// bridge. The adapter derives its own stable opaque per-intent token.
		reply := h.callWithMetadata(req, map[string]any{
			"idempotency":     maliciousIdempotency,
			"correlation_id":  confWireOtherMail,
			"resource_id":     confWireOtherEVP,
			"private_context": map[string]any{"value": confWireNested},
		})
		h.decrypt(req, reply)
		h.assertSafeEvent(reply, req, i)
		h.assertPrivateAbsent(reply.raw)
	}
	h.assertPrivateAbsent(h.logs.Bytes())
	_, _, puts, events := h.counts()
	if puts != 2 || events != 2 {
		t.Fatal("metadata override changed the two canonical actual PUT/event attempts")
	}
	var first, second struct {
		Idempotency string `json:"idempotency"`
	}
	h.mu.Lock()
	firstRaw, secondRaw := append([]byte(nil), h.events[0]...), append([]byte(nil), h.events[1]...)
	h.mu.Unlock()
	if json.Unmarshal(firstRaw, &first) != nil || json.Unmarshal(secondRaw, &second) != nil || first.Idempotency != second.Idempotency {
		t.Fatal("same intent acquired a different event idempotency on repeated delivery")
	}
}

func TestConfidentialSelectedInstanceTLSFailureIsStaticBeforeProviderAccess(t *testing.T) {
	for _, purpose := range []string{"list", "ensure"} {
		t.Run(purpose, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			t.Setenv("EFI_CERTIFICATE", "")
			t.Setenv("EFI_CERTIFICATE_BASE64", "")
			req := h.request(purpose)
			envelope := h.envelope(req.Binding.Operation, h.input(req))
			instance := envelope["integration"].(map[string]any)["instance_spec"].(map[string]any)
			instance["config"].(map[string]any)["mtls_enabled"] = true
			instance["credentials"].(map[string]any)["efi_certificate_base64"] = "%%%" + confWireNested
			reply := h.callRaw(confWireJSON(t, envelope), h.receiver)
			if reply.response.OK || reply.response.Error == nil || reply.response.Error.Message != confidential.ErrHeld.Error() {
				t.Fatal("selected-instance TLS failure did not become a static confidential error")
			}
			calls, oauth, puts, events := h.counts()
			if calls != 0 || oauth != 0 || puts != 0 || events != 0 {
				t.Fatal("unusable selected-instance certificate reached OAuth/provider/event I/O")
			}
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
		})
	}
}

func TestLegacyConfidentialLiteralValuesDoNotEnableSealedMode(t *testing.T) {
	h := newConfidentialWireHarness(t)
	const literal = "confidential_input"
	h.provider = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/webhook/"+literal {
			t.Error("legacy literal input did not retain its original read")
		}
		confWireWrite(w, confWireRecord(literal, confWireBase))
	}
	envelope := h.envelope(adapter.OperationObserveWebhookSubscriptions, map[string]any{
		"chave":         literal,
		"legacy_note":   literal,
		"legacy_nested": map[string]any{"confidential_input": literal},
	})
	envelope["metadata"] = map[string]any{"confidential_input": literal, "idempotency": literal}
	reply := h.callRaw(confWireJSON(t, envelope), h.receiver)
	if !reply.response.OK || reply.legacy["chave"] != literal {
		t.Fatal("a legacy value or unrelated nested/metadata name was treated as confidential opt-in")
	}
	if _, sealed := reply.legacy["privacy"]; sealed {
		t.Fatal("legacy output was unexpectedly replaced with a confidential envelope")
	}
	calls, oauth, puts, events := h.counts()
	if calls != 1 || oauth != 1 || puts != 0 || events != 0 {
		t.Fatal("legacy compatibility read caused extra calls or mutation")
	}
}

func TestConfidentialAuditIdentityCannotExposeAnyProviderKey(t *testing.T) {
	for _, name := range []string{"target-key-alias", "other-listed-evp-alias"} {
		t.Run(name, func(t *testing.T) {
			h := newConfidentialWireHarness(t)
			req := h.request("ensure")
			wantReads := 0
			req.Binding.AuditResourceID = confWireTarget
			if name == "other-listed-evp-alias" {
				req.Binding.AuditResourceID = confWireOtherEVP
				wantReads = 1
			}
			h.provider = func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v2/webhook" {
					t.Error("a private key used as public audit identity reached mutation or confirmation")
				}
				confWireWrite(w, confWirePage(confWireOthers()))
			}
			reply := h.call(req)
			if reply.response.OK || reply.response.Error == nil {
				t.Fatal("a provider key was accepted as a public audit identity")
			}
			calls, oauth, puts, events := h.counts()
			if calls != wantReads || oauth != wantReads || puts != 0 || events != 0 {
				t.Fatal("audit alias refusal occurred after an unexpected provider/event operation")
			}
			h.assertPrivateAbsent(reply.raw)
			h.assertPrivateAbsent(h.logs.Bytes())
		})
	}
}

func TestConfidentialAcceptedPUTCanFinishAndConfirmAfterAuthorizationExpiry(t *testing.T) {
	h := newConfidentialWireHarness(t)
	req := h.request("ensure")
	expires, err := time.Parse(time.RFC3339Nano, req.Binding.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	h.provider = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/webhook":
			confWireWrite(w, confWirePage(confWireOthers()))
		case r.Method == http.MethodPut && r.URL.Path == "/v2/webhook/"+confWireTarget:
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["webhookUrl"] != confWireBase {
				t.Error("expiry-boundary setter changed its canonical receiver")
			}
			// Authorization was valid before send. Expire it at the accepted
			// setter, while retaining the original handler context for GET,
			// sealing and SDK event emission after this response.
			h.setTime(expires)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/webhook/"+confWireTarget:
			if h.now().Before(expires) {
				t.Error("readback did not exercise the expired authorization boundary")
			}
			record := confWireRecord(confWireTarget, confWireBase)
			record["unknown"] = map[string]any{"private": confWireNested}
			confWireWrite(w, record)
		default:
			t.Error("unexpected expiry-boundary provider operation")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	ensured := h.call(req)
	plain := h.decrypt(req, ensured)
	if !h.now().Equal(expires) || ensured.output.Proof.Outcome != "confirmed" || ensured.output.Proof.Ensured == nil || !*ensured.output.Proof.Ensured || ensured.output.Proof.Noop == nil || *ensured.output.Proof.Noop || len(plain.GuardConfirmation) == 0 {
		t.Fatal("accepted PUT could not finish its original GET/seal/event lifecycle at exact expiry")
	}
	calls, oauth, puts, events := h.counts()
	if calls != 3 || oauth != 3 || puts != 1 || events != 1 {
		t.Fatal("exact-expiry completion lost a read/event or repeated the setter")
	}
	h.assertSafeEvent(ensured, req, 0)
	h.assertPrivateAbsent(ensured.raw)

	// A separately delivered read-only final confirmation carries the SAME
	// expired authorization interval. Opening this result is not a new PUT
	// authorization and must not require a regenerated recipient or key.
	h.setTime(expires.Add(time.Second))
	confirm := h.request("confirm")
	confirm.Binding.Mode = req.Binding.Mode
	confirm.Binding.IssuedAt = req.Binding.IssuedAt
	confirm.Binding.ExpiresAt = req.Binding.ExpiresAt
	confirmed := h.call(confirm)
	confirmation := h.decrypt(confirm, confirmed)
	var legacy map[string]any
	if confirmed.output.Proof.Kind != "single" || confirmed.output.Proof.Outcome != "confirmed" || json.Unmarshal(confirmation.LegacyOutput, &legacy) != nil || legacy["chave"] != confWireTarget || legacy["webhookUrl"] != confWireBase {
		t.Fatal("expired purpose-confirm did not retain its exact sealed read-only DTO")
	}
	calls, oauth, puts, events = h.counts()
	if calls != 4 || oauth != 4 || puts != 1 || events != 1 {
		t.Fatal("expired purpose-confirm added a PUT/event or failed to perform exactly one GET")
	}
	h.assertPrivateAbsent(confirmed.raw)
	h.assertPrivateAbsent(h.logs.Bytes())
}
