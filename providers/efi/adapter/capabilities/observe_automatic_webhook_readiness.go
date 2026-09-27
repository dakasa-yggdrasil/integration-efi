package capabilities

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	automaticWebhookEnsuredEvent        = "efi.automatic_webhook.ensured"
	automaticWebhookEventPrincipalID    = "integration-efi"
	automaticWebhookEventGrantCount     = 13
	automaticWebhookEventGrantSetSHA256 = "01de7f37235b2b4d306f8092cb3f5655c0e1fa929265b5ea07c86e7a5c030a14"
	maxCoreAuthorizationResponseBytes   = 32 * 1024
)

// AutomaticWebhookReadinessConfig comes only from the registered integration
// instance and adapter environment. In particular, ReceiverURL and EventToken
// are never accepted as capability inputs.
type AutomaticWebhookReadinessConfig struct {
	ReceiverURL        string
	ProviderBaseURL    string
	TLSConfig          *tls.Config
	OAuthAuthenticated bool
	CoreURL            string
	EventToken         string
	InstanceID         string
}

type automaticWebhookAuthorization struct {
	Allowed            bool   `json:"allowed"`
	PrincipalID        string `json:"principal_id"`
	Provider           string `json:"provider"`
	InstanceID         string `json:"instance_id"`
	EventType          string `json:"event_type"`
	GrantForm          string `json:"grant_form"`
	GrantCount         int    `json:"grant_count"`
	GrantSetSHA256     string `json:"grant_set_sha256"`
	PrincipalExpiresAt string `json:"principal_expires_at"`
}

type automaticWebhookReadinessDeps struct {
	resolveAddresses  func(context.Context, string) ([]string, error)
	clientlessProbe   func(context.Context, string, string, *tls.Config) error
	authenticatedPost func(context.Context, string, string, *tls.Config) (int, string, error)
	authorizeEvent    func(context.Context, string, string, string) (automaticWebhookAuthorization, error)
}

// ObserveAutomaticWebhookReadiness proves the live, read-only prerequisites
// for automatic-webhook registration. The only caller-controlled field is the
// load-balancer DNS name observed from the orchestrator; the receiver URL,
// production P12 and Core event bearer stay in their existing trusted config
// boundaries.
func ObserveAutomaticWebhookReadiness(ctx context.Context, cfg AutomaticWebhookReadinessConfig, in map[string]any) (map[string]any, error) {
	deps := automaticWebhookReadinessDeps{
		resolveAddresses:  resolvePublicAddresses,
		clientlessProbe:   requireClientlessTLSRefusal,
		authenticatedPost: postAuthenticatedRegistrationProbe,
		authorizeEvent:    observeCoreEventAuthorization,
	}
	return observeAutomaticWebhookReadiness(ctx, cfg, in, deps)
}

func observeAutomaticWebhookReadiness(ctx context.Context, cfg AutomaticWebhookReadinessConfig, in map[string]any, deps automaticWebhookReadinessDeps) (map[string]any, error) {
	if len(in) != 1 {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: expected only expected_dns_name")
	}
	expectedDNSName, ok := in["expected_dns_name"].(string)
	if !ok {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: expected_dns_name is required")
	}
	expectedDNSName, err := normalizedDNSName(expectedDNSName)
	if err != nil {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: invalid expected_dns_name")
	}
	receiverURL, receiverHost, receiverPort, err := normalizedReceiverURL(cfg.ReceiverURL)
	if err != nil {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: configured receiver URL is invalid")
	}
	if cfg.TLSConfig == nil || len(cfg.TLSConfig.Certificates) == 0 {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: EFI client certificate is unavailable")
	}
	if !cfg.OAuthAuthenticated {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: EFI OAuth authentication was not proven")
	}
	instanceID := strings.TrimSpace(cfg.InstanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: integration instance is required")
	}
	providerBaseURL, err := normalizedProviderBaseURL(cfg.ProviderBaseURL)
	if err != nil {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: configured EFI base URL is invalid")
	}
	clientCertificateSHA256, err := clientLeafCertificateSHA256(cfg.TLSConfig)
	if err != nil {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: EFI client certificate fingerprint is unavailable")
	}

	receiverAddresses, err := deps.resolveAddresses(ctx, receiverHost)
	if err != nil || len(receiverAddresses) == 0 {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: receiver DNS lookup failed")
	}
	expectedAddresses, err := deps.resolveAddresses(ctx, expectedDNSName)
	if err != nil || len(expectedAddresses) == 0 {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: expected load balancer DNS lookup failed")
	}
	if !equalStringSets(receiverAddresses, expectedAddresses) {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: receiver DNS does not match the expected load balancer")
	}
	addressProbes := make([]map[string]any, 0, len(receiverAddresses))
	peerCertificateSet := make(map[string]struct{}, len(receiverAddresses))
	for _, address := range receiverAddresses {
		dialAddress := net.JoinHostPort(address, receiverPort)
		if err := deps.clientlessProbe(ctx, dialAddress, receiverHost, cfg.TLSConfig); err != nil {
			return nil, fmt.Errorf("observe_automatic_webhook_readiness: clientless mTLS refusal was not proven for every receiver address")
		}
		status, peerCertificateSHA256, err := deps.authenticatedPost(ctx, receiverURL, dialAddress, cfg.TLSConfig)
		if err != nil || status != http.StatusOK || len(peerCertificateSHA256) != sha256.Size*2 {
			return nil, fmt.Errorf("observe_automatic_webhook_readiness: authenticated registration probe was not accepted by every receiver address")
		}
		peerCertificateSHA256 = strings.ToLower(peerCertificateSHA256)
		if _, err := hex.DecodeString(peerCertificateSHA256); err != nil {
			return nil, fmt.Errorf("observe_automatic_webhook_readiness: authenticated registration probe returned an invalid certificate fingerprint")
		}
		peerCertificateSet[peerCertificateSHA256] = struct{}{}
		addressProbes = append(addressProbes, map[string]any{
			"address":                          address,
			"clientless_certificate_requested": true,
			"clientless_refused":               true,
			"authenticated_status":             status,
			"peer_certificate_sha256":          peerCertificateSHA256,
		})
	}
	peerCertificateSHA256s := make([]string, 0, len(peerCertificateSet))
	for fingerprint := range peerCertificateSet {
		peerCertificateSHA256s = append(peerCertificateSHA256s, fingerprint)
	}
	sort.Strings(peerCertificateSHA256s)
	authorization, err := deps.authorizeEvent(ctx, cfg.CoreURL, cfg.EventToken, instanceID)
	if err != nil {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: Core event authorization proof failed")
	}
	principalExpiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(authorization.PrincipalExpiresAt))
	if err != nil || !principalExpiresAt.After(time.Now().UTC()) {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: Core event principal expiry was invalid")
	}
	principalExpiresAtValue := principalExpiresAt.UTC().Format(time.RFC3339)
	if !authorization.Allowed || authorization.PrincipalID != automaticWebhookEventPrincipalID || authorization.Provider != "efi" ||
		authorization.InstanceID != instanceID || authorization.EventType != automaticWebhookEnsuredEvent ||
		authorization.GrantForm != "exact" ||
		authorization.GrantCount != automaticWebhookEventGrantCount ||
		authorization.GrantSetSHA256 != automaticWebhookEventGrantSetSHA256 {
		return nil, fmt.Errorf("observe_automatic_webhook_readiness: Core event authorization proof was incomplete")
	}

	technicalIdentity := map[string]any{
		"provider":                  "efi",
		"instance_id":               instanceID,
		"provider_base_url":         providerBaseURL,
		"oauth_authenticated":       true,
		"client_certificate_sha256": clientCertificateSHA256,
	}
	technicalIdentity["evidence_sha256"] = canonicalEvidenceSHA256(technicalIdentity)
	webhookEvidence := map[string]any{
		"receiver_url":               receiverURL,
		"receiver_host":              receiverHost,
		"expected_dns_name":          expectedDNSName,
		"receiver_addresses":         receiverAddresses,
		"expected_addresses":         expectedAddresses,
		"dns_matches_expected":       true,
		"all_addresses_proven":       true,
		"address_probe_count":        len(addressProbes),
		"address_probes":             addressProbes,
		"peer_certificate_sha256s":   peerCertificateSHA256s,
		"event_principal_id":         authorization.PrincipalID,
		"event_grant_form":           authorization.GrantForm,
		"event_grant_count":          authorization.GrantCount,
		"event_grant_set_sha256":     authorization.GrantSetSHA256,
		"event_principal_expires_at": principalExpiresAtValue,
	}
	webhookEvidence["evidence_sha256"] = canonicalEvidenceSHA256(webhookEvidence)

	return map[string]any{
		"ready":                            true,
		"receiver_url":                     receiverURL,
		"receiver_host":                    receiverHost,
		"expected_dns_name":                expectedDNSName,
		"receiver_addresses":               receiverAddresses,
		"expected_addresses":               expectedAddresses,
		"dns_matches_expected":             true,
		"all_addresses_proven":             true,
		"address_probe_count":              len(addressProbes),
		"address_probes":                   addressProbes,
		"clientless_certificate_requested": true,
		"clientless_refused":               true,
		"authenticated_status":             http.StatusOK,
		"peer_certificate_sha256s":         peerCertificateSHA256s,
		"technical_identity_evidence":      technicalIdentity,
		"webhook_evidence":                 webhookEvidence,
		"commercial_eligibility": map[string]any{
			"status": "not_observed",
			"reason": "OAuth, mTLS, DNS and event authorization prove technical readiness only",
		},
		"account_identity": map[string]any{
			"status": "not_observed",
			"reason": "the EFI adapter contract exposes no read-only authoritative account identity",
		},
		"event_authorization": map[string]any{
			"allowed":              authorization.Allowed,
			"principal_id":         authorization.PrincipalID,
			"provider":             authorization.Provider,
			"instance_id":          authorization.InstanceID,
			"event_type":           authorization.EventType,
			"grant_form":           authorization.GrantForm,
			"grant_count":          authorization.GrantCount,
			"grant_set_sha256":     authorization.GrantSetSHA256,
			"principal_expires_at": principalExpiresAtValue,
		},
	}, nil
}

func normalizedProviderBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("invalid provider base URL")
	}
	return value, nil
}

func clientLeafCertificateSHA256(config *tls.Config) (string, error) {
	if config == nil || len(config.Certificates) == 0 || len(config.Certificates[0].Certificate) == 0 {
		return "", errors.New("client certificate unavailable")
	}
	digest := sha256.Sum256(config.Certificates[0].Certificate[0])
	return hex.EncodeToString(digest[:]), nil
}

func canonicalEvidenceSHA256(value map[string]any) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func normalizedDNSName(value string) (string, error) {
	value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	parsed, err := url.Parse("https://" + value)
	if value == "" || net.ParseIP(value) != nil || strings.ContainsAny(value, "/:@?#") ||
		err != nil || parsed.Hostname() != value || parsed.Port() != "" || parsed.Path != "" {
		return "", errors.New("invalid DNS name")
	}
	return value, nil
}

func normalizedReceiverURL(value string) (normalized, host, port string, err error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || parsed.Path == "" || parsed.Path == "/" {
		return "", "", "", errors.New("invalid receiver URL")
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return "", "", "", errors.New("invalid receiver port")
	}
	host, err = normalizedDNSName(parsed.Hostname())
	if err != nil {
		return "", "", "", err
	}
	port = parsed.Port()
	if port == "" {
		port = "443"
	}
	return parsed.String(), host, port, nil
}

func resolvePublicAddresses(ctx context.Context, host string) ([]string, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if !address.IP.IsGlobalUnicast() || address.IP.IsPrivate() || address.IP.IsLoopback() ||
			address.IP.IsLinkLocalUnicast() || address.IP.IsLinkLocalMulticast() || address.IP.IsMulticast() {
			return nil, errors.New("DNS answer is not publicly routable")
		}
		set[address.IP.String()] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for address := range set {
		result = append(result, address)
	}
	sort.Strings(result)
	return result, nil
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func requireClientlessTLSRefusal(ctx context.Context, address, serverName string, base *tls.Config) error {
	config := base.Clone()
	config.ServerName = serverName
	config.Certificates = nil
	requested := false
	config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		requested = true
		return &tls.Certificate{}, nil
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: config}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || !requested {
		return errors.New("server did not require a client certificate")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errors.New("TLS handshake timed out")
	}
	return nil
}

func postAuthenticatedRegistrationProbe(ctx context.Context, receiverURL, address string, base *tls.Config) (int, string, error) {
	parsed, err := url.Parse(receiverURL)
	if err != nil {
		return 0, "", err
	}
	tlsConfig := base.Clone()
	tlsConfig.ServerName = parsed.Hostname()
	netDialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.TLSClientConfig = tlsConfig
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return netDialer.DialContext(ctx, network, address)
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, receiverURL, http.NoBody)
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
		return response.StatusCode, "", errors.New("peer certificate unavailable")
	}
	digest := sha256.Sum256(response.TLS.PeerCertificates[0].Raw)
	return response.StatusCode, hex.EncodeToString(digest[:]), nil
}

func observeCoreEventAuthorization(ctx context.Context, coreURL, eventToken, instanceID string) (automaticWebhookAuthorization, error) {
	coreURL = strings.TrimSpace(coreURL)
	eventToken = strings.TrimSpace(eventToken)
	parsed, err := url.Parse(coreURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || eventToken == "" {
		return automaticWebhookAuthorization{}, errors.New("Core authorization endpoint is unavailable")
	}
	body, _ := json.Marshal(map[string]string{
		"provider":    "efi",
		"instance_id": instanceID,
		"event_type":  automaticWebhookEnsuredEvent,
	})
	endpoint := strings.TrimRight(coreURL, "/") + "/api/v1/events/authorization"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return automaticWebhookAuthorization{}, errors.New("build Core authorization request")
	}
	request.Header.Set("Authorization", "Bearer "+eventToken)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return automaticWebhookAuthorization{}, errors.New("Core authorization request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return automaticWebhookAuthorization{}, fmt.Errorf("Core authorization rejected with status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxCoreAuthorizationResponseBytes+1))
	if err != nil || len(raw) > maxCoreAuthorizationResponseBytes {
		return automaticWebhookAuthorization{}, errors.New("read Core authorization response")
	}
	var result automaticWebhookAuthorization
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return automaticWebhookAuthorization{}, errors.New("decode Core authorization response")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return automaticWebhookAuthorization{}, errors.New("decode complete Core authorization response")
	}
	return result, nil
}
