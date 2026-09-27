package capabilities

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestObserveAutomaticWebhookReadinessChainsDNSClientlessMTLSAndEffectiveGrant(t *testing.T) {
	var calls []string
	deps := automaticWebhookReadinessDeps{
		resolveAddresses: func(_ context.Context, host string) ([]string, error) {
			calls = append(calls, "dns:"+host)
			return []string{"203.0.113.10", "203.0.113.11"}, nil
		},
		clientlessProbe: func(_ context.Context, receiverURL, address string, _ *tls.Config) (string, error) {
			calls = append(calls, "clientless:"+receiverURL+":"+address)
			if strings.HasPrefix(address, "203.0.113.10") {
				return strings.Repeat("a", sha256.Size*2), nil
			}
			return strings.Repeat("b", sha256.Size*2), nil
		},
		authorizeEvent: func(_ context.Context, coreURL, token, instance string) (automaticWebhookAuthorization, error) {
			calls = append(calls, fmt.Sprintf("authorization:%s:%s:%s", coreURL, token, instance))
			return automaticWebhookAuthorization{
				Allowed: true, PrincipalID: "integration-efi", Provider: "efi", InstanceID: instance,
				EventType: automaticWebhookEnsuredEvent, GrantForm: "exact", GrantCount: 13,
				GrantSetSHA256: automaticWebhookEventGrantSetSHA256, PrincipalExpiresAt: "2099-01-01T01:00:00+01:00",
			}, nil
		},
	}
	output, err := observeAutomaticWebhookReadiness(context.Background(), AutomaticWebhookReadinessConfig{
		ReceiverURL:        "https://webhook-pix.dakasa.me/payment/webhook/efi",
		ProviderBaseURL:    "https://pix.api.efipay.com.br",
		TLSConfig:          &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}},
		OAuthAuthenticated: true,
		CoreURL:            "http://yggdrasil.dakasa.svc.cluster.local:9080",
		EventToken:         "super-secret-bearer",
		InstanceID:         "efi-dakasa-production",
	}, map[string]any{"expected_dns_name": "nlb.example.net"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if output["ready"] != true || output["dns_matches_expected"] != true || output["all_addresses_proven"] != true || output["address_probe_count"] != 2 || output["clientless_refused"] != true {
		t.Fatalf("output=%v", output)
	}
	addressProbes := output["address_probes"].([]map[string]any)
	if len(addressProbes) != 2 {
		t.Fatalf("address coverage=%v", output)
	}
	for index, addressProbe := range addressProbes {
		if addressProbe["authenticated_status"] != 0 {
			t.Fatalf("address probe did not retain the fail-closed legacy status: %v", addressProbe)
		}
		wantFingerprint := strings.Repeat(string(rune('a'+index)), sha256.Size*2)
		if addressProbe["peer_certificate_sha256"] != wantFingerprint {
			t.Fatalf("address probe receiver certificate fingerprint=%v want=%v", addressProbe, wantFingerprint)
		}
	}
	providerProbe := output["provider_authenticated_probe"].(map[string]any)
	if providerProbe["status"] != "not_observed" || providerProbe["reason"] != "provider-authenticated callback acceptance is proven only when EFI processes the webhook registration PUT" {
		t.Fatalf("provider probe=%v", providerProbe)
	}
	if output["authenticated_status"] != 0 {
		t.Fatalf("output did not retain the fail-closed legacy status: %v", output)
	}
	wantFingerprints := []string{strings.Repeat("a", sha256.Size*2), strings.Repeat("b", sha256.Size*2)}
	if !reflect.DeepEqual(output["peer_certificate_sha256s"], wantFingerprints) {
		t.Fatalf("receiver certificate fingerprints=%v want=%v", output["peer_certificate_sha256s"], wantFingerprints)
	}
	webhookEvidence := output["webhook_evidence"].(map[string]any)
	if !reflect.DeepEqual(webhookEvidence["provider_authenticated_probe"], providerProbe) {
		t.Fatalf("sealed webhook evidence omitted the pending provider probe: %v", webhookEvidence)
	}
	if !reflect.DeepEqual(webhookEvidence["peer_certificate_sha256s"], wantFingerprints) {
		t.Fatalf("sealed webhook evidence omitted receiver certificate fingerprints: %v", webhookEvidence)
	}
	authorization := output["event_authorization"].(map[string]any)
	if authorization["principal_id"] != "integration-efi" || authorization["grant_count"] != 13 || authorization["event_type"] != automaticWebhookEnsuredEvent {
		t.Fatalf("authorization=%v", authorization)
	}
	if authorization["principal_expires_at"] != "2099-01-01T00:00:00Z" {
		t.Fatalf("expiry was not normalized: %v", authorization)
	}
	if strings.Contains(fmt.Sprint(output), "super-secret-bearer") {
		t.Fatalf("output exposed bearer: %v", output)
	}
	if output["commercial_eligibility"].(map[string]any)["status"] != "not_observed" || output["account_identity"].(map[string]any)["status"] != "not_observed" {
		t.Fatalf("technical proof was promoted to commercial/account evidence: %v", output)
	}
	if len(output["technical_identity_evidence"].(map[string]any)["evidence_sha256"].(string)) != 64 || len(webhookEvidence["evidence_sha256"].(string)) != 64 {
		t.Fatalf("evidence seals are absent: %v", output)
	}
	wantCalls := []string{
		"dns:webhook-pix.dakasa.me", "dns:nlb.example.net",
		"clientless:https://webhook-pix.dakasa.me/payment/webhook/efi:203.0.113.10:443",
		"clientless:https://webhook-pix.dakasa.me/payment/webhook/efi:203.0.113.11:443",
		"authorization:http://yggdrasil.dakasa.svc.cluster.local:9080:super-secret-bearer:efi-dakasa-production",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
	}
}

func TestRequireClientlessTLSRefusalReadsRequiredCertificateAlert(t *testing.T) {
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			var handlerCalled atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				handlerCalled.Store(true)
			}))
			server.TLS = &tls.Config{
				MinVersion: version,
				MaxVersion: version,
				ClientAuth: tls.RequireAndVerifyClientCert,
			}
			server.StartTLS()
			defer server.Close()

			base := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			var verifyConnectionCalled atomic.Bool
			base.VerifyConnection = func(tls.ConnectionState) error {
				verifyConnectionCalled.Store(true)
				return nil
			}
			fingerprint, err := requireClientlessTLSRefusal(context.Background(), server.URL, server.Listener.Addr().String(), base)
			if err != nil {
				t.Fatalf("required client certificate was not proven: %v", err)
			}
			if !verifyConnectionCalled.Load() {
				t.Fatal("existing VerifyConnection callback was not preserved")
			}
			serverFingerprint := sha256.Sum256(server.Certificate().Raw)
			if fingerprint != hex.EncodeToString(serverFingerprint[:]) {
				t.Fatalf("receiver certificate fingerprint=%q want=%q", fingerprint, hex.EncodeToString(serverFingerprint[:]))
			}
			if handlerCalled.Load() {
				t.Fatal("clientless request reached the HTTP handler")
			}
		})
	}
}

func TestRequireClientlessTLSRefusalRejectsOptionalCertificateRequest(t *testing.T) {
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			var handlerCalled atomic.Bool
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				handlerCalled.Store(true)
				w.WriteHeader(http.StatusNoContent)
			}))
			server.TLS = &tls.Config{
				MinVersion: version,
				MaxVersion: version,
				ClientAuth: tls.RequestClientCert,
			}
			server.StartTLS()
			defer server.Close()

			base := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			_, err := requireClientlessTLSRefusal(context.Background(), server.URL, server.Listener.Addr().String(), base)
			if err == nil || !handlerCalled.Load() {
				t.Fatalf("optional client certificate was accepted as refusal: err=%v handlerCalled=%v", err, handlerCalled.Load())
			}
		})
	}
}

func TestRequiredClientCertificateRefusalMatchesAlertToTLSVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		version uint16
		err     error
		want    bool
	}{
		{name: "TLS 1.3 certificate required", version: tls.VersionTLS13, err: &net.OpError{Op: "remote error", Err: errors.New("tls: certificate required")}, want: true},
		{name: "TLS 1.3 generic handshake failure", version: tls.VersionTLS13, err: &net.OpError{Op: "remote error", Err: errors.New("tls: handshake failure")}},
		{name: "TLS 1.2 handshake failure", version: tls.VersionTLS12, err: &net.OpError{Op: "remote error", Err: errors.New("tls: handshake failure")}, want: true},
		{name: "TLS 1.2 certificate required", version: tls.VersionTLS12, err: &net.OpError{Op: "remote error", Err: errors.New("tls: certificate required")}},
		{name: "internal TLS error", version: tls.VersionTLS13, err: &net.OpError{Op: "remote error", Err: errors.New("tls: internal error")}},
		{name: "connection reset", version: tls.VersionTLS13, err: errors.New("connection reset by peer")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isRequiredClientCertificateRefusal(test.err, test.version); got != test.want {
				t.Fatalf("isRequiredClientCertificateRefusal() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRequireClientlessTLSRefusalRejectsCanceledRequest(t *testing.T) {
	handlerStarted := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(handlerStarted)
		<-request.Context().Done()
	}))
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		ClientAuth: tls.RequestClientCert,
	}
	server.StartTLS()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-handlerStarted
		cancel()
	}()
	base := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if _, err := requireClientlessTLSRefusal(ctx, server.URL, server.Listener.Addr().String(), base); err == nil {
		t.Fatal("canceled request was accepted as client-certificate refusal")
	}
}

func TestObserveAutomaticWebhookReadinessRejectsUnexpectedGrantSet(t *testing.T) {
	for _, authorization := range []automaticWebhookAuthorization{
		{
			Allowed: true, PrincipalID: "integration-efi", Provider: "efi", InstanceID: "efi-dakasa-production",
			EventType: automaticWebhookEnsuredEvent, GrantForm: "exact", GrantCount: automaticWebhookEventGrantCount - 1,
			GrantSetSHA256: automaticWebhookEventGrantSetSHA256, PrincipalExpiresAt: "2027-03-15T00:00:00Z",
		},
		{
			Allowed: true, PrincipalID: "integration-efi", Provider: "efi", InstanceID: "efi-dakasa-production",
			EventType: automaticWebhookEnsuredEvent, GrantForm: "exact", GrantCount: automaticWebhookEventGrantCount,
			GrantSetSHA256: strings.Repeat("b", 64), PrincipalExpiresAt: "2027-03-15T00:00:00Z",
		},
		{
			Allowed: true, PrincipalID: "integration-efi", Provider: "efi", InstanceID: "efi-dakasa-production",
			EventType: automaticWebhookEnsuredEvent, GrantForm: "exact", GrantCount: automaticWebhookEventGrantCount,
			GrantSetSHA256: automaticWebhookEventGrantSetSHA256, PrincipalExpiresAt: "not-rfc3339",
		},
		{
			Allowed: true, PrincipalID: "integration-efi", Provider: "efi", InstanceID: "efi-dakasa-production",
			EventType: automaticWebhookEnsuredEvent, GrantForm: "exact", GrantCount: automaticWebhookEventGrantCount,
			GrantSetSHA256: automaticWebhookEventGrantSetSHA256, PrincipalExpiresAt: "2000-01-01T00:00:00Z",
		},
	} {
		deps := automaticWebhookReadinessDeps{
			resolveAddresses: func(context.Context, string) ([]string, error) {
				return []string{"203.0.113.10"}, nil
			},
			clientlessProbe: func(context.Context, string, string, *tls.Config) (string, error) {
				return strings.Repeat("a", sha256.Size*2), nil
			},
			authorizeEvent: func(context.Context, string, string, string) (automaticWebhookAuthorization, error) {
				return authorization, nil
			},
		}
		_, err := observeAutomaticWebhookReadiness(context.Background(), AutomaticWebhookReadinessConfig{
			ReceiverURL:        "https://webhook-pix.dakasa.me/payment/webhook/efi",
			ProviderBaseURL:    "https://pix.api.efipay.com.br",
			TLSConfig:          &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}},
			OAuthAuthenticated: true,
			InstanceID:         "efi-dakasa-production",
		}, map[string]any{"expected_dns_name": "nlb.example.net"}, deps)
		if err == nil {
			t.Fatalf("accepted unexpected grant set: %+v", authorization)
		}
	}
}

func TestObserveAutomaticWebhookReadinessFailsWhenAnyResolvedAddressIsUnproven(t *testing.T) {
	authorizationCalled := false
	deps := automaticWebhookReadinessDeps{
		resolveAddresses: func(context.Context, string) ([]string, error) {
			return []string{"203.0.113.10", "203.0.113.11"}, nil
		},
		clientlessProbe: func(_ context.Context, _, address string, _ *tls.Config) (string, error) {
			if strings.HasPrefix(address, "203.0.113.11") {
				return "", errors.New("second address accepted clientless TLS")
			}
			return strings.Repeat("a", sha256.Size*2), nil
		},
		authorizeEvent: func(context.Context, string, string, string) (automaticWebhookAuthorization, error) {
			authorizationCalled = true
			return automaticWebhookAuthorization{}, nil
		},
	}
	_, err := observeAutomaticWebhookReadiness(context.Background(), AutomaticWebhookReadinessConfig{
		ReceiverURL:        "https://webhook-pix.dakasa.me/payment/webhook/efi",
		ProviderBaseURL:    "https://pix.api.efipay.com.br",
		TLSConfig:          &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}},
		OAuthAuthenticated: true,
		InstanceID:         "efi-dakasa-production",
	}, map[string]any{"expected_dns_name": "nlb.example.net"}, deps)
	if err == nil || authorizationCalled {
		t.Fatalf("err=%v authorizationCalled=%v", err, authorizationCalled)
	}
}

func TestObserveAutomaticWebhookReadinessFailsClosedBeforeCoreAuthorization(t *testing.T) {
	for _, test := range []struct {
		name        string
		receiver    []string
		expected    []string
		fingerprint string
		clientless  error
	}{
		{name: "dns mismatch", receiver: []string{"203.0.113.10"}, expected: []string{"203.0.113.11"}},
		{name: "clientless accepted", receiver: []string{"203.0.113.10"}, expected: []string{"203.0.113.10"}, clientless: errors.New("not refused")},
		{name: "invalid receiver certificate fingerprint", receiver: []string{"203.0.113.10"}, expected: []string{"203.0.113.10"}, fingerprint: "not-a-sha256"},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizationCalled := false
			lookup := 0
			deps := automaticWebhookReadinessDeps{
				resolveAddresses: func(context.Context, string) ([]string, error) {
					lookup++
					if lookup == 1 {
						return test.receiver, nil
					}
					return test.expected, nil
				},
				clientlessProbe: func(context.Context, string, string, *tls.Config) (string, error) {
					fingerprint := test.fingerprint
					if fingerprint == "" {
						fingerprint = strings.Repeat("a", sha256.Size*2)
					}
					return fingerprint, test.clientless
				},
				authorizeEvent: func(context.Context, string, string, string) (automaticWebhookAuthorization, error) {
					authorizationCalled = true
					return automaticWebhookAuthorization{}, errors.New("must not run")
				},
			}
			_, err := observeAutomaticWebhookReadiness(context.Background(), AutomaticWebhookReadinessConfig{
				ReceiverURL:        "https://webhook-pix.dakasa.me/payment/webhook/efi",
				ProviderBaseURL:    "https://pix.api.efipay.com.br",
				TLSConfig:          &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}},
				OAuthAuthenticated: true,
				InstanceID:         "efi-dakasa-production",
			}, map[string]any{"expected_dns_name": "nlb.example.net"}, deps)
			if err == nil || authorizationCalled {
				t.Fatalf("err=%v authorizationCalled=%v", err, authorizationCalled)
			}
		})
	}
}

func TestObserveAutomaticWebhookReadinessRejectsDestinationOverride(t *testing.T) {
	_, err := observeAutomaticWebhookReadiness(context.Background(), AutomaticWebhookReadinessConfig{}, map[string]any{
		"expected_dns_name": "nlb.example.net",
		"receiver_url":      "https://attacker.example/collect",
	}, automaticWebhookReadinessDeps{})
	if err == nil || !strings.Contains(err.Error(), "expected only expected_dns_name") {
		t.Fatalf("destination override err=%v", err)
	}
}

func TestObserveCoreEventAuthorizationRefusesRedirects(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled = true
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	_, err := observeCoreEventAuthorization(context.Background(), redirect.URL, "sensitive-bearer", "efi-dakasa-production")
	if err == nil || targetCalled || strings.Contains(err.Error(), "sensitive-bearer") {
		t.Fatalf("err=%v targetCalled=%v", err, targetCalled)
	}
}

func TestObserveCoreEventAuthorizationRequiresOneBoundedJSONDocument(t *testing.T) {
	for _, body := range []string{
		`{"allowed":true,"principal_id":"integration-efi","provider":"efi","instance_id":"efi-dakasa-production","event_type":"efi.automatic_webhook.ensured","grant_form":"exact","grant_count":13,"grant_set_sha256":"` + strings.Repeat("b", 64) + `","principal_expires_at":"2027-03-15T00:00:00Z","unexpected":true}`,
		`{"allowed":true} {"allowed":true}`,
		strings.Repeat(" ", maxCoreAuthorizationResponseBytes) + `{}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		_, err := observeCoreEventAuthorization(context.Background(), server.URL, "sensitive-bearer", "efi-dakasa-production")
		server.Close()
		if err == nil {
			t.Fatalf("accepted invalid response of %d bytes", len(body))
		}
	}
}
