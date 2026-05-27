package capabilities

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/config"
	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

func TestDestroyCharge_PatchesToCobWithRemovedStatus(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := DestroyCharge(context.Background(), c, map[string]any{"txid": "tx-A"})
	if err != nil {
		t.Fatalf("DestroyCharge = %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/v2/cob/tx-A" {
		t.Fatalf("path = %q, want /v2/cob/tx-A", gotPath)
	}
	if !strings.Contains(string(gotBody), `"status":"REMOVIDA_PELO_USUARIO_RECEBEDOR"`) {
		t.Fatalf("body missing remove status: %s", string(gotBody))
	}
	if got["destroyed"] != true {
		t.Fatalf("destroyed = %v", got["destroyed"])
	}
}

func TestDestroyCharge_404IsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "t"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := efiapi.NewEfiClient(config.Config{ClientKeyID: "k", ClientSecret: "s", BaseURL: srv.URL, MTLSEnabled: false}, nil)
	got, err := DestroyCharge(context.Background(), c, map[string]any{"txid": "tx-gone"})
	if err != nil {
		t.Fatalf("DestroyCharge with 404 = %v, want nil (idempotent)", err)
	}
	if got["destroyed"] != true {
		t.Fatalf("destroyed = %v, want true on 404", got["destroyed"])
	}
	if got["already_absent"] != true {
		t.Fatalf("already_absent = %v, want true on 404", got["already_absent"])
	}
}

func TestDestroyCharge_RequiresTxid(t *testing.T) {
	c := &efiapi.EfiClient{}
	_, err := DestroyCharge(context.Background(), c, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "txid") {
		t.Fatalf("expected txid required, got %v", err)
	}
}
