package adapter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/mtls"
)

func TestNewWebhookServer_ClonesTLSConfigForInboundPolicy(t *testing.T) {
	roots := x509.NewCertPool()
	clientRoots := x509.NewCertPool()
	original := &tls.Config{
		RootCAs:    roots,
		ClientCAs:  clientRoots,
		MinVersion: tls.VersionTLS13,
	}

	server := NewWebhookServer(":0", original, nil, zap.NewNop())
	if server.tlsConfig == original {
		t.Fatal("inbound server retained the caller's tls.Config pointer")
	}
	if original.ClientAuth != tls.NoClientCert {
		t.Fatalf("caller config ClientAuth mutated to %v", original.ClientAuth)
	}
	if server.tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("server ClientAuth = %v, want RequireAndVerifyClientCert", server.tlsConfig.ClientAuth)
	}
	if server.tlsConfig.RootCAs != roots {
		t.Fatal("server clone lost outbound RootCAs")
	}
	if server.tlsConfig.ClientCAs != clientRoots {
		t.Fatal("server clone lost caller-provided inbound ClientCAs")
	}
	if server.tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("server MinVersion = %x, want preserved TLS 1.3", server.tlsConfig.MinVersion)
	}
	if server.srv.TLSConfig != server.tlsConfig {
		t.Fatal("http.Server does not use the isolated inbound tls.Config")
	}
}

func TestNewWebhookServer_MissingDedicatedClientCAsFailsClosed(t *testing.T) {
	original := &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS12}
	server := NewWebhookServer(":0", original, nil, zap.NewNop())
	if original.ClientCAs != nil {
		t.Fatal("caller config was mutated")
	}
	if server.tlsConfig.ClientCAs == nil {
		t.Fatal("server retained nil ClientCAs and would fall back to host roots")
	}
	if len(server.tlsConfig.ClientCAs.Subjects()) != 0 {
		t.Fatal("missing dedicated EFI CA must produce an empty fail-closed pool")
	}
}

func TestWebhookServer_ListenAndServeReportsStartupFailureImmediately(t *testing.T) {
	webhook := NewWebhookServer(":0", &tls.Config{
		ClientCAs:  x509.NewCertPool(),
		MinVersion: tls.VersionTLS12,
	}, nil, nil)

	result := make(chan error, 1)
	go func() {
		result <- webhook.ListenAndServe(context.Background())
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("startup unexpectedly succeeded without a server certificate")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe hid an immediate startup failure until context cancellation")
	}
}

func TestWebhookServer_InboundMTLSAcceptsTrustedClientCertificate(t *testing.T) {
	fixture, err := mtls.Load(mtls.Config{
		Source: mtls.SourceFile,
		Path:   filepath.Join("testdata", "test.p12"),
	})
	if err != nil {
		t.Fatalf("load test client certificate: %v", err)
	}
	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(fixture.Certificates[0].Leaf)

	webhook := NewWebhookServer(":0", &tls.Config{
		ClientCAs:  clientRoots,
		MinVersion: tls.VersionTLS12,
	}, nil, zap.NewNop())
	tlsServer := httptest.NewUnstartedServer(webhook.srv.Handler)
	tlsServer.TLS = webhook.tlsConfig
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)

	transport := tlsServer.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.Certificates = fixture.Certificates
	client := &http.Client{Transport: transport}

	resp, err := client.Get(tlsServer.URL + "/efi/webhook/pix")
	if err != nil {
		t.Fatalf("trusted mTLS request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestWebhookServer_InboundMTLSRejectsMissingClientCertificate(t *testing.T) {
	clientRoots := x509.NewCertPool()
	webhook := NewWebhookServer(":0", &tls.Config{
		ClientCAs:  clientRoots,
		MinVersion: tls.VersionTLS12,
	}, nil, zap.NewNop())
	tlsServer := httptest.NewUnstartedServer(webhook.srv.Handler)
	tlsServer.TLS = webhook.tlsConfig
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)

	resp, err := tlsServer.Client().Get(tlsServer.URL + "/efi/webhook/pix")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("request without client certificate unexpectedly returned %d", resp.StatusCode)
	}
}
