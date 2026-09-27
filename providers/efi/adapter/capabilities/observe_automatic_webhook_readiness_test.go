package capabilities

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestObserveAutomaticWebhookReadinessChainsDNSMTLSAndEffectiveGrant(t *testing.T) {
	var calls []string
	deps := automaticWebhookReadinessDeps{
		resolveAddresses: func(_ context.Context, host string) ([]string, error) {
			calls = append(calls, "dns:"+host)
			return []string{"203.0.113.10", "203.0.113.11"}, nil
		},
		clientlessProbe: func(_ context.Context, address, serverName string, _ *tls.Config) error {
			calls = append(calls, "clientless:"+address+":"+serverName)
			return nil
		},
		authenticatedPost: func(_ context.Context, receiverURL, address string, _ *tls.Config) (int, string, error) {
			calls = append(calls, "authenticated:"+receiverURL+":"+address)
			return 200, strings.Repeat("a", 64), nil
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
	if output["ready"] != true || output["dns_matches_expected"] != true || output["all_addresses_proven"] != true || output["address_probe_count"] != 2 || output["clientless_refused"] != true || output["authenticated_status"] != 200 {
		t.Fatalf("output=%v", output)
	}
	if len(output["address_probes"].([]map[string]any)) != 2 || len(output["peer_certificate_sha256s"].([]string)) != 1 {
		t.Fatalf("address coverage=%v", output)
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
	if len(output["technical_identity_evidence"].(map[string]any)["evidence_sha256"].(string)) != 64 || len(output["webhook_evidence"].(map[string]any)["evidence_sha256"].(string)) != 64 {
		t.Fatalf("evidence seals are absent: %v", output)
	}
	wantCalls := []string{
		"dns:webhook-pix.dakasa.me", "dns:nlb.example.net",
		"clientless:203.0.113.10:443:webhook-pix.dakasa.me",
		"authenticated:https://webhook-pix.dakasa.me/payment/webhook/efi:203.0.113.10:443",
		"clientless:203.0.113.11:443:webhook-pix.dakasa.me",
		"authenticated:https://webhook-pix.dakasa.me/payment/webhook/efi:203.0.113.11:443",
		"authorization:http://yggdrasil.dakasa.svc.cluster.local:9080:super-secret-bearer:efi-dakasa-production",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls=%v want=%v", calls, wantCalls)
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
			clientlessProbe: func(context.Context, string, string, *tls.Config) error { return nil },
			authenticatedPost: func(context.Context, string, string, *tls.Config) (int, string, error) {
				return http.StatusOK, strings.Repeat("a", 64), nil
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
		clientlessProbe: func(_ context.Context, address, _ string, _ *tls.Config) error {
			if strings.HasPrefix(address, "203.0.113.11") {
				return errors.New("second address accepted clientless TLS")
			}
			return nil
		},
		authenticatedPost: func(context.Context, string, string, *tls.Config) (int, string, error) {
			return http.StatusOK, strings.Repeat("a", 64), nil
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

func TestObserveAutomaticWebhookReadinessFailsClosedBeforeAuthenticatedProbe(t *testing.T) {
	for _, test := range []struct {
		name       string
		receiver   []string
		expected   []string
		clientless error
	}{
		{name: "dns mismatch", receiver: []string{"203.0.113.10"}, expected: []string{"203.0.113.11"}},
		{name: "clientless accepted", receiver: []string{"203.0.113.10"}, expected: []string{"203.0.113.10"}, clientless: errors.New("not refused")},
	} {
		t.Run(test.name, func(t *testing.T) {
			postCalled := false
			lookup := 0
			deps := automaticWebhookReadinessDeps{
				resolveAddresses: func(context.Context, string) ([]string, error) {
					lookup++
					if lookup == 1 {
						return test.receiver, nil
					}
					return test.expected, nil
				},
				clientlessProbe: func(context.Context, string, string, *tls.Config) error { return test.clientless },
				authenticatedPost: func(context.Context, string, string, *tls.Config) (int, string, error) {
					postCalled = true
					return 200, strings.Repeat("a", 64), nil
				},
				authorizeEvent: func(context.Context, string, string, string) (automaticWebhookAuthorization, error) {
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
			if err == nil || postCalled {
				t.Fatalf("err=%v postCalled=%v", err, postCalled)
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
