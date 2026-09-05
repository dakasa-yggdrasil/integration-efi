package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestProductionEmitErrorDoesNotExposeResponseOrToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"must-not-escape"}`))
	}))
	t.Cleanup(server.Close)

	emit := newProductionEmitFunc(server.URL, "test-token", zap.NewNop())
	err := emit(context.Background(), "events", "efi.pix.received", map[string]any{
		"private": "payload-must-not-escape",
	})
	if err == nil {
		t.Fatal("expected publish error")
	}
	for _, sensitive := range []string{"must-not-escape", "payload-must-not-escape", "test-token"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("publish error leaked %q: %v", sensitive, err)
		}
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("publish error lost safe status context: %v", err)
	}
}
