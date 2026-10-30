package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type webhookWireResult struct {
	response rpcResponse
	output   map[string]any
	raw      []byte
	logs     *bytes.Buffer
}

// This runs the same handler, registered SDK reconcilers and real default
// adapter dispatch used by RabbitMQ. Only the external provider is a mock;
// no operation, client constructor or per-request credential path is stubbed.
func webhookWireCall(t *testing.T, input map[string]any, status int, body []byte, capabilityOnly bool) webhookWireResult {
	t.Helper()
	providerCalls, oauthCalls := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			oauthCalls++
			key, secret, ok := r.BasicAuth()
			if !ok || key != "instance-client-key" || secret != "instance-client-secret" {
				t.Error("provider did not receive the selected instance credentials")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "fixture-provider-token"})
			return
		}
		providerCalls++
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer fixture-provider-token" || r.Header.Get("x-skip-mtls-checking") != "" {
			t.Error("observation changed provider method, authentication or mTLS skip semantics")
		}
		if key, single := input["chave"].(string); single {
			if r.URL.EscapedPath() != "/v2/webhook/"+url.PathEscape(strings.TrimSpace(key)) || r.URL.RawQuery != "" {
				t.Error("exact key input changed before the provider read")
			}
		} else {
			if r.URL.Path != "/v2/webhook" || r.URL.Query().Get("inicio") != input["inicio"] || r.URL.Query().Get("fim") != input["fim"] || r.URL.Query().Get("paginacao.itensPorPagina") != fmt.Sprint(input["page_size"]) || r.URL.Query().Get("paginacao.paginaAtual") != fmt.Sprint(input["page"]) {
				t.Error("provider did not receive the exact requested list page")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(provider.Close)
	events := 0
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { events++; w.WriteHeader(http.StatusAccepted) }))
	t.Cleanup(core.Close)
	t.Setenv("YGGDRASIL_CORE_URL", core.URL)
	t.Setenv("YGGDRASIL_RUN_TOKEN", "fixture-event-token")
	t.Setenv("EFI_API_CLIENT_KEY_ID", "wrong-environment-key")
	t.Setenv("EFI_API_CLIENT_SECRET", "wrong-environment-secret")
	t.Setenv("EFI_BASE_URL", "https://environment-must-not-be-used.invalid")
	a := sdkadapter.New(sdkadapter.Config{Provider: adapter.Provider, IntegrationType: adapter.IntegrationType, Version: adapter.AdapterVersion})
	adapter.WireReconcilers(a, "wrong-fallback-instance")
	logs := &bytes.Buffer{}
	logCore := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(logs), zap.ErrorLevel)
	handler := ExecuteHandler(zap.New(logCore), a)
	request := map[string]any{"operation": adapter.OperationObserveWebhookSubscriptions, "capability": adapter.OperationObserveWebhookSubscriptions, "input": input, "integration": map[string]any{"instance": map[string]any{"name": "selected-fixture-instance"}, "instance_spec": map[string]any{"credentials": map[string]any{"efi_client_key_id": "instance-client-key", "efi_client_secret": "instance-client-secret"}, "config": map[string]any{"base_url": provider.URL, "mtls_enabled": false}}}}
	if capabilityOnly {
		request["operation"] = ""
	}
	encoded, _ := json.Marshal(request)
	raw, contentType, err := handler(context.Background(), rpc.Delivery{Body: encoded, ContentType: "application/json"})
	if err != nil || contentType != "application/json" {
		t.Fatal("actual handler did not return its RPC envelope", err)
	}
	var response rpcResponse
	if json.Unmarshal(raw, &response) != nil {
		t.Fatal("actual handler returned invalid JSON")
	}
	var data struct {
		Operation  string         `json:"operation"`
		Capability string         `json:"capability"`
		Output     map[string]any `json:"output"`
		Metadata   map[string]any `json:"metadata"`
	}
	if response.OK {
		encoded, _ := json.Marshal(response.Data)
		if json.Unmarshal(encoded, &data) != nil || data.Operation != request["operation"] || data.Capability != request["capability"] {
			t.Fatal("read response did not preserve inbound operation/capability")
		}
		if _, exists := data.Metadata["base_url"]; exists {
			t.Fatal("read response exposed unrelated instance configuration")
		}
	}
	if providerCalls != 1 || oauthCalls != 1 || events != 0 {
		t.Fatal("read performed extra provider operations or emitted a mutation event")
	}
	return webhookWireResult{response: response, output: data.Output, raw: raw, logs: logs}
}

func webhookWireFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	var out map[string]any
	if err != nil || json.Unmarshal(raw, &out) != nil {
		t.Fatal("cannot read deterministic wire fixture")
	}
	return out
}

func webhookProviderList(t *testing.T) map[string]any {
	t.Helper()
	wire := webhookWireFixture(t, "webhook-observation-list.v1.json")
	return map[string]any{"parametros": wire["parametros"], "webhooks": wire["webhooks"]}
}

func webhookListInput() map[string]any {
	return map[string]any{"inicio": "2020-11-16T00:00:00.000Z", "fim": "2026-10-30T12:00:00.000Z", "page": 0, "page_size": 1000}
}

func TestExecuteHandlerWebhookObservationPreservesSingleAndListWireFixtures(t *testing.T) {
	for _, single := range []bool{true, false} {
		t.Run(fmt.Sprint(single), func(t *testing.T) {
			name := "webhook-observation-list.v1.json"
			input := webhookListInput()
			provider := webhookProviderList(t)
			if single {
				name = "webhook-observation-single.v1.json"
				fixture := webhookWireFixture(t, name)
				provider = map[string]any{"chave": fixture["chave"], "webhookUrl": fixture["webhookUrl"], "criacao": fixture["criacao"]}
				input = map[string]any{"chave": fixture["chave"]}
			}
			body, _ := json.Marshal(provider)
			result := webhookWireCall(t, input, 200, body, false)
			if !result.response.OK || !reflect.DeepEqual(result.output, webhookWireFixture(t, name)) {
				t.Fatal("actual default-dispatch wire result differs from its public fixture")
			}
		})
	}
}

func TestExecuteHandlerWebhookObservationPreservesCapabilityOnlyAndEscapedKey(t *testing.T) {
	key := "fixture+alias@example.test"
	body, _ := json.Marshal(map[string]any{"chave": key, "webhookUrl": "https://receiver.example.test/webhook", "criacao": "2026-10-30T12:00:00Z"})
	result := webhookWireCall(t, map[string]any{"chave": key}, 200, body, true)
	if !result.response.OK || result.output["chave"] != key {
		t.Fatal("capability-only or exact-key observation changed")
	}
}

func TestExecuteHandlerWebhookPageProvenanceRejectsIncompleteOrAmbiguousPages(t *testing.T) {
	for _, name := range []string{"missing parameters", "missing pagination", "missing count", "string count", "fractional count", "wrong page", "wrong size", "wrong range", "count mismatch", "missing records", "nonobject record", "duplicate key", "noncanonical uuid", "urn uuid", "missing key", "missing url", "nonhttps url", "userinfo url", "fragment url", "empty fragment url", "invalid query url", "missing created", "early created", "late created", "bad created", "null pagination"} {
		t.Run(name, func(t *testing.T) {
			provider := webhookProviderList(t)
			parameters := provider["parametros"].(map[string]any)
			pagination := parameters["paginacao"].(map[string]any)
			rows := provider["webhooks"].([]any)
			row := rows[0].(map[string]any)
			switch name {
			case "missing parameters":
				delete(provider, "parametros")
			case "missing pagination":
				delete(parameters, "paginacao")
			case "null pagination":
				parameters["paginacao"] = nil
			case "missing count":
				delete(pagination, "quantidadeTotalDeItens")
			case "string count":
				pagination["quantidadeTotalDeItens"] = "2"
			case "fractional count":
				pagination["quantidadeTotalDeItens"] = 2.5
			case "wrong page":
				pagination["paginaAtual"] = 1
			case "wrong size":
				pagination["itensPorPagina"] = 10
			case "wrong range":
				parameters["fim"] = "2026-10-31T12:00:00Z"
			case "count mismatch":
				pagination["quantidadeTotalDeItens"] = 3
			case "missing records":
				delete(provider, "webhooks")
			case "nonobject record":
				rows[0] = "not a record"
			case "duplicate key":
				rows[1].(map[string]any)["chave"] = row["chave"]
			case "noncanonical uuid":
				row["chave"] = "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF"
			case "urn uuid":
				row["chave"] = "urn:uuid:11111111-1111-4111-8111-111111111111"
			case "missing key":
				delete(row, "chave")
			case "missing url":
				delete(row, "webhookUrl")
			case "nonhttps url":
				row["webhookUrl"] = "http://receiver.example.test/webhook"
			case "userinfo url":
				row["webhookUrl"] = "https://private-user:private-password@receiver.example.test/webhook"
			case "fragment url":
				row["webhookUrl"] = "https://receiver.example.test/webhook#fragment"
			case "empty fragment url":
				row["webhookUrl"] = "https://receiver.example.test/webhook#"
			case "invalid query url":
				row["webhookUrl"] = "https://receiver.example.test/webhook?hmac=%ZZ"
			case "missing created":
				delete(row, "criacao")
			case "early created":
				row["criacao"] = "2020-11-15T23:59:59Z"
			case "late created":
				row["criacao"] = "2026-10-30T12:00:01Z"
			case "bad created":
				row["criacao"] = "not a date"
			}
			body, _ := json.Marshal(provider)
			result := webhookWireCall(t, webhookListInput(), 200, body, false)
			if !result.response.OK {
				t.Fatal("malformed raw provider page was not preserved for diagnosis")
			}
			proof, ok := result.output["page_provenance"].(map[string]any)
			if !ok || proof["schemaVersion"] != float64(1) || proof["validated"] != false || proof["complete"] != false {
				t.Fatal("malformed original page gained completion proof")
			}
			if name == "missing parameters" {
				if _, exists := result.output["parametros"]; exists {
					t.Fatal("missing original provider parameters were fabricated")
				}
			}
			if name == "duplicate key" && len(result.output["webhooks"].([]any)) != 2 {
				t.Fatal("duplicate raw records were silently removed")
			}
		})
	}
}

func TestExecuteHandlerWebhookObservationEmptyBodyDoesNotInventResource(t *testing.T) {
	result := webhookWireCall(t, map[string]any{"chave": "fixture-key"}, 200, nil, false)
	if !result.response.OK || result.output["items"] != nil || result.output["cursor"] != "" {
		t.Fatal("empty single observation changed compatibility envelope")
	}
	for _, name := range []string{"chave", "webhookUrl", "criacao", "page_provenance"} {
		if _, exists := result.output[name]; exists {
			t.Fatal("empty single observation invented provider fields or proof")
		}
	}
}

func TestExecuteHandlerWebhookSingleRefusesUnsafeOriginalURLBeforeSanitizing(t *testing.T) {
	for _, address := range []any{
		"http://receiver.example.test/webhook",
		"https://private-user:private-password@receiver.example.test/webhook",
		"https://receiver.example.test/webhook#private-fragment",
		"https://receiver.example.test/webhook#",
		"https://receiver.example.test/webhook?hmac=private-query%ZZ",
		nil,
		42,
	} {
		body, _ := json.Marshal(map[string]any{"chave": "fixture-key", "webhookUrl": address, "criacao": "2026-10-30T12:00:00.000Z"})
		result := webhookWireCall(t, map[string]any{"chave": "fixture-key"}, 200, body, false)
		if result.response.OK || result.response.Error == nil || result.response.Error.Code != "execute_failed" {
			t.Fatal("unsafe original single URL gained a sanitized success observation")
		}
		for _, secret := range []string{"fixture-key", "private-user", "private-password", "private-fragment", "private-query", "instance-client-secret"} {
			if strings.Contains(string(result.raw), secret) || strings.Contains(result.logs.String(), secret) {
				t.Fatal("unsafe original single URL leaked into RPC errors or logs")
			}
		}
	}
}

func TestExecuteHandlerWebhookObservationKeepsPagedResultAndRedactsAliases(t *testing.T) {
	provider := webhookProviderList(t)
	provider["parametros"].(map[string]any)["paginacao"] = map[string]any{"paginaAtual": 0, "itensPorPagina": 2, "quantidadeDePaginas": 2, "quantidadeTotalDeItens": 3}
	provider["webhooks"].([]any)[0].(map[string]any)["webhookUrl"] = "https://receiver.example.test/webhook?hmac=private-query-canary"
	body, _ := json.Marshal(provider)
	input := webhookListInput()
	input["page_size"] = 2
	result := webhookWireCall(t, input, 200, body, false)
	proof, _ := result.output["page_provenance"].(map[string]any)
	if !result.response.OK || proof["validated"] != true || proof["complete"] != false || result.output["cursor"] != "1" {
		t.Fatal("valid paginated observation was hidden or promoted to a complete list")
	}
	if strings.Contains(string(result.raw), "private-query-canary") || !reflect.DeepEqual(result.output["items"], result.output["webhooks"]) {
		t.Fatal("raw/compatibility aliases leaked query secrets or disagreed")
	}
}

func TestExecuteHandlerWebhookObservationRefusesProviderErrorsAndDuplicateJSONWithoutBodyLeak(t *testing.T) {
	for _, status := range []int{400, 403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := []byte(`{"detail":"private-provider-canary fixture-key private-query-canary"}`)
			result := webhookWireCall(t, map[string]any{"chave": "fixture-key"}, status, body, false)
			if result.response.OK || result.response.Error == nil || result.response.Error.Code != "execute_failed" {
				t.Fatal("provider error became a successful observation or absence")
			}
			for _, secret := range []string{"private-provider-canary", "fixture-key", "private-query-canary", "instance-client-secret"} {
				if strings.Contains(string(result.raw), secret) {
					t.Fatal("provider detail leaked into the RPC error")
				}
				if strings.Contains(result.logs.String(), secret) {
					t.Fatal("provider detail leaked into logs")
				}
			}
		})
	}
	for _, body := range []string{`{"parametros":{"paginacao":{"paginaAtual":0,"paginaAtual":1}},"webhooks":[]}`, `{"webhooks":[{"chave":"private-json-canary","chave":"other-key"}]}`, `{"webhooks":[],"Webhooks":[]}`} {
		result := webhookWireCall(t, webhookListInput(), 200, []byte(body), false)
		if result.response.OK || strings.Contains(string(result.raw), "private-json-canary") {
			t.Fatal("ambiguous provider JSON was accepted or reflected")
		}
	}
}
