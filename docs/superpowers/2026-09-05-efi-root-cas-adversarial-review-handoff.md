# EFI adapter TLS, webhook contract, and secret-safety review handoff

Date: 2026-09-05

Worktree: `/Users/dakasa/projects/dakasa/.codex-worktrees/integration-efi-system-roots`

Branch: `codex/efi-system-roots-20260905`

Base reviewed: `153234d`

No commit, push, deploy, workflow registration, EFI API call, provider mutation, secret read, or certificate deletion was performed.

## Decision

**GO for one review PR and a build candidate. BLOCK for EFI production activation.**

There is a scoped **GO** for the outbound TLS root repair, TLS 1.2 floor, webhook URL registration semantics, webhook observation contract, the repaired surface caller, URL and error secret boundaries, and fail-closed behavior when the embedded listener has no dedicated EFI client CA. The final focused suite, full Go suite, critical race suite, production surface build, local image build, diff check, and credential-pattern scan all pass.

The remaining production block is architectural and configuration-related:

1. DaKasa production webhook ingestion belongs to `dakasa-identities`, not to this integration adapter. This follows the canonical integration contract and DaKasa ADR-0172.
2. The adapter's embedded listener has no input for the dedicated EFI webhook CA chain and no separate server certificate/key input. It currently reuses the outbound EFI API P12 as a server certificate.
3. The advertised `verify_webhook_signature` helper parses dates and compares an issuer CN, but does not verify a certificate chain, signature, EKU, or full validity interval. It must not be used as an authentication boundary.
4. No live provider observation, registered-resource proof, authenticated callback, image deployment, or registered-version proof was produced in this worktree.

Therefore this branch is safe to review and build, but it is not sufficient evidence to register the production webhook or declare EFI live. Production activation remains gated on the dedicated `dakasa-identities` receiver, the production EFI CA at that receiver, and an authenticated callback proof.

## Exact DaKasa production URL contract

For the dedicated mTLS production receiver, register this base URL with EFI:

```text
https://webhook.dakasa.me/efi/webhook
```

EFI sends its registration test to the exact registered URL. For normal real Pix notifications without the query-parameter escape form, EFI appends `/pix`, so the final delivery URL is:

```text
https://webhook.dakasa.me/efi/webhook/pix
```

The DaKasa identities service exposes both POST routes and is the authoritative business receiver. Homologation uses the separate `webhook-h.dakasa.me` host and homologation credentials and CA.

EFI also documents the shared-hosting escape form `?hmac=<value>&ignorar=`. In that form the appended `/pix` is consumed by the `ignorar` query value instead of becoming a path segment. This review keeps that form syntactically valid, redacts its query values from observations, and does not recommend it for the dedicated DaKasa mTLS production route.

The adapter now rejects:

- non-HTTPS URLs;
- relative URLs;
- URLs with userinfo or a fragment;
- URLs whose path already ends in `/pix`, including a trailing slash variant.

This prevents the common `/pix/pix` registration mistake. The exact production base above is covered by the capability test.

Primary EFI source: <https://dev.efipay.com.br/en/docs/api-pix/webhooks/>

## Confirmed P0 repair: outbound SystemCertPool and TLS 1.2

The vendored SDK P12 loader creates a non-nil but empty `RootCAs` pool. Go only falls back to host roots when `RootCAs` is nil. A non-nil empty pool rejects the normal EFI server chain before OAuth can complete.

`LoadTLSConfig` now:

1. decodes the configured file or base64 P12 as before;
2. calls `x509.SystemCertPool()`;
3. fails fast when that call errors or returns nil;
4. replaces the SDK's empty pool with the host pool;
5. sets `MinVersion` to `tls.VersionTLS12`.

Tests cover file and base64 sources, the exact installed pool, the TLS floor, system-pool failure, nil-pool failure, disabled mTLS, and missing certificate input. The client documentation was also corrected: a nil client TLS config omits the client certificate but the default Go transport still verifies the provider server certificate.

## Inbound listener findings

`RootCAs` and `ClientCAs` serve different trust directions:

- `RootCAs` verifies EFI's API server for outbound calls.
- `ClientCAs` verifies the client certificate EFI presents to the inbound webhook receiver.

`NewWebhookServer` previously reused and mutated the caller's `*tls.Config`. It now clones the config before installing `RequireAndVerifyClientCert`.

When `ClientCAs` is absent, the clone now receives an explicit empty pool. This fails closed instead of allowing Go to fall back to broad system roots for inbound client authentication. Tests prove:

- the caller's TLS config is not mutated;
- `RootCAs`, a caller-supplied `ClientCAs`, and TLS version survive the clone;
- absent dedicated `ClientCAs` becomes an explicit empty pool;
- a client signed by the configured test CA is accepted;
- a request without a client certificate is rejected;
- listener startup errors return immediately instead of remaining hidden until context cancellation.

This is intentionally safe but not sufficient for production. The real receiver must trust only the correct environment-specific EFI webhook chain:

- production: `https://certificados.efipay.com.br/webhooks/certificate-chain-prod.crt`
- homologation: `https://certificados.efipay.com.br/webhooks/certificate-chain-homolog.crt`

It must also present a real hostname-valid server certificate and private key. The provider-issued outbound P12 is not evidence of server-auth EKU or hostname identity.

DaKasa's accepted topology already provides the correct ownership boundary: Traefik TLS passthrough to the dedicated TLS listener in `dakasa-identities`. Port 9079 in this adapter remains a compatibility and local-test path, not a production-ready receiver.

## Webhook observer repair

The former empty-filter path sent `GET /v2/webhook` without query parameters. EFI documents `inicio` and `fim` as mandatory and the live endpoint had returned HTTP 400 for the empty request.

The observer now supports two unambiguous forms:

```text
{chave}
{inicio, fim[, page, page_size, cursor]}
```

Behavior now covered by tests:

- `chave` must be a non-empty string;
- key lookup and time-range fields are mutually exclusive;
- the Pix key is path-escaped, so it cannot inject a provider query or path segment;
- `inicio` and `fim` must both be valid RFC3339 strings;
- `fim` must be equal to or later than `inicio`;
- `page` must be a non-negative integer;
- `page_size` must be an integer from 1 through 1000;
- `cursor` is a validated integer string and is mutually exclusive with `page`;
- adapter pagination names map to `paginacao.paginaAtual` and `paginacao.itensPorPagina`;
- the next zero-based provider page is returned as the top-level cursor;
- the cursor is empty on the last page.

No implicit moving window was added inside the adapter. A short rolling window can silently hide an older but still active webhook and falsely report that no subscription exists. The surface caller now supplies an explicit full-history range with a fixed `2020-01-01T00:00:00.000Z` lower bound and the read time as `fim`, which covers every possible Pix-era subscription. Its params are memoized for the mounted query so render cycles cannot continuously change the query key and refetch.

## Secret and private-value boundaries

The hardening now prevents several routes by which external values could enter RPC errors, logs, traces, workflow history, or the surface:

1. Observed `webhookUrl` and `url` values have every non-empty query value redacted. Query key names remain visible for drift diagnosis.
2. The surface only receives the already-sanitized URL.
3. EFI non-2xx response bodies are no longer forwarded at all. Only HTTP status and a caller-owned static error name cross the boundary. This is stronger than heuristic redaction because provider validation messages can reflect arbitrary Pix keys, URLs, HMACs, or credentials.
4. OAuth failures use a static message.
5. Network errors preserve `errors.Is` and `errors.As` via `Unwrap`, but their public `Error()` string omits URL, path, Pix key, query, token, and underlying transport detail.
6. OpenTelemetry spans no longer record the raw HTTP path. They record only the bounded operation class.
7. Errors returned by the adapter-to-core `publish_message` path no longer include the core response body, request payload, or bearer token.

Tests use sentinel strings and assert that none escape. No real credential value is present in the tests or this handoff.

## Surface state and caller are now honest

EFI's documented GET response does not expose the registration-time mTLS flag. The previous projection defaulted an absent field to `true`, which could display a healthy state without evidence.

The projection and UI now preserve three states:

- `true`: explicitly observed mTLS;
- `false`: explicitly observed skip-mTLS;
- `null`: EFI did not report the registration mode.

The UI labels `null` as `não observado`, keeps it distinct from `sem mTLS`, and prioritizes confirmed skip-mTLS rows. The live data hook no longer calls the provider with an invalid empty filter: it uses the explicit Pix-era history window described above. The charge and webhook hooks also memoize their timestamped params so ordinary rerenders do not create a moving query key. The production TypeScript/Vite build passes.

## Remaining security blocker: verify_webhook_signature

`providers/efi/adapter/capabilities/verify_webhook_signature.go` returns `valid: true` for any parseable, not-yet-expired certificate when `expected_issuer` is omitted. Even with that field, it only compares the issuer Common Name. A forged certificate can copy a Common Name.

Until this helper requires and verifies a trusted EFI CA bundle with `x509.Verify`, the correct client-auth EKU, `NotBefore`, `NotAfter`, and the full presented chain, it must be treated as informational only or removed from the advertised capability catalog. The TLS handshake in the authoritative backend receiver is the actual security boundary.

## Files changed in this worktree

Go runtime and tests:

- `cmd/adapter/main.go`
- `cmd/adapter/main_test.go` (new)
- `providers/efi/efiapi/client.go`
- `providers/efi/efiapi/client_test.go`
- `providers/efi/adapter/mtls.go`
- `providers/efi/adapter/mtls_test.go`
- `providers/efi/adapter/webhook_server.go`
- `providers/efi/adapter/webhook_server_test.go` (new)
- `providers/efi/adapter/capabilities/webhook_url.go` (new)
- `providers/efi/adapter/capabilities/ensure_webhook_subscription.go`
- `providers/efi/adapter/capabilities/ensure_webhook_subscription_test.go`
- `providers/efi/adapter/capabilities/observe_webhook_subscriptions.go`
- `providers/efi/adapter/capabilities/observe_webhook_subscriptions_test.go`
- `providers/efi/adapter/capabilities/destroy_webhook_subscription.go`
- `providers/efi/adapter/surface_query.go`
- `providers/efi/adapter/surface_query_test.go`
- `providers/efi/adapter/spec.go`

Contracts and operator documentation:

- `manifest/capabilities/ensure_webhook_subscription.yaml`
- `manifest/capabilities/observe_webhook_subscriptions.yaml`
- `docs/CAPABILITIES.md`
- `docs/USAGE.md`
- `yggdrasil-quickstart.yaml`
- `docs/superpowers/2026-09-05-efi-root-cas-adversarial-review-handoff.md`

Surface:

- `surface-ui/src/data/index.ts`
- `surface-ui/src/data/mock.ts`
- `surface-ui/src/data/types.ts`
- `surface-ui/src/data/useCharges.ts`
- `surface-ui/src/data/useEfiPulse.ts`
- `surface-ui/src/data/useWebhookSubscriptions.ts`
- `surface-ui/src/screens/Home.tsx`
- `surface-ui/src/screens/Webhook.tsx`
- `surface-ui/src/screens/home-parts/AttentionBand.tsx`
- `surface-ui/src/screens/home-parts/KpiStrip.tsx`
- `surface-ui/src/screens/webhook-parts/WebhookTable.tsx`

## Validation evidence

Passed focused Go packages:

```text
GOWORK=off go test ./providers/efi/efiapi ./providers/efi/adapter/capabilities ./providers/efi/adapter ./cmd/adapter -count=1
ok all four packages
```

Passed key observer regression:

```text
GOWORK=off go test ./providers/efi/adapter/capabilities -run 'TestObserveWebhookSubscriptions' -count=1
ok github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities
```

Passed race detector on the critical packages:

```text
GOMAXPROCS=2 GOWORK=off go test -race -p=1 ./providers/efi/adapter/capabilities ./providers/efi/adapter ./providers/efi/efiapi ./cmd/adapter -count=1
ok all four packages
```

Passed complete repository Go suite:

```text
GOMAXPROCS=2 GOWORK=off go test -p=1 ./... -count=1
ok all packages
```

Passed production surface build on the final surface sources:

```text
npm --prefix surface-ui run build
tsc --noEmit passed
vite build passed, 1007 modules transformed
```

The first direct dependency install attempt failed with HTTP 401 because the current GitHub Packages token is unauthorized. No package lock or node_modules directory was left behind. The build was then run through a temporary symlink to the existing `surface-eco` workspace dependency tree and passed; the symlink and generated `dist` directory were removed afterward.

Passed local image build:

```text
docker build -t integration-efi:local .
image integration-efi:local built successfully
```

Compose config validation could not complete because this isolated worktree has no `.env`; no `.env` was created and no secret or placeholder was printed.

Final hygiene checks passed:

```text
git diff --check
passed

changed/untracked credential-pattern scan
34 files scanned; no AWS, GitHub, Stripe live key, private-key PEM, JWT, or literal long-secret assignment pattern found

generated-output check
no surface-ui/node_modules symlink and no surface-ui/dist directory left behind
```

## Required next actions

1. Keep the outbound SystemCertPool and TLS 1.2 repair.
2. Keep the exact production registration base and do not register the final `/pix` route.
3. Use a modern production-scoped EFI instance and a guarded ensure plus read-only observe workflow, pinned to the correct provider tier and Pix key. Do not infer provider registration from ingress existence.
4. Keep `skip_mtls_validation` false for the dedicated DaKasa receiver.
5. Prove the exact provider-side subscription using the single-key observer, then prove an EFI-authenticated callback at the backend receiver.
6. Decide whether to delete or explicitly deprecate the embedded adapter listener and reactor. If retained, add separate environment-specific `ClientCAs` and server certificate/key inputs first.
7. Harden or remove `verify_webhook_signature` before anyone relies on it.
8. Bump binary, registered manifest, image tag, and workflow expectations consistently before release.
9. Preserve all production destroy operations behind exact identity, tier guards, and explicit human approval.
