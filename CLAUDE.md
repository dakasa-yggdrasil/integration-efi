# Claude Code Context: integration-efi

A standalone Yggdrasil **integration adapter** for the EFI (formerly
Gerencianet) **Brazilian Pix** payments provider. It speaks the Banco
Central PIX API (`/v2/cob`, `/v2/cobv`, `/v2/pix`, `/v2/webhook`) plus
EFI's `/v3/gn` overlay, authenticating with OAuth client-credentials
over **mTLS**, and turns inbound EFI Pix webhook callbacks into events
on the bus via a reactor.

- Repo: `github.com/dakasa-yggdrasil/integration-efi` (Apache 2.0).
- `integration_type` / provider: **`efi`** (single-provider family, so
  type == provider). Manifest `namespace`: **`global`**. Domain:
  `payments`.
- Read `README.md` and the `docs/` suite (`USAGE`, `CONFIGURATION`,
  `CAPABILITIES`, `OPERATIONS`, `DEVELOPMENT`,
  `RUNBOOK_STAGING_VALIDATION`) for the operator-facing detail.

> **Trust `Describe()` in `providers/efi/adapter/spec.go` over this
> file.** `spec.go` is the authoritative contract — capabilities,
> credential/instance schemas, resource types, transport, and
> `AdapterVersion` are all defined there and asserted by `spec_test.go`
> + the `pkg/contractcheck` lint (run under `go test ./...`). If this
> file and `spec.go` ever disagree, `spec.go` wins; fix this file.

## Repo layout (real)

```
cmd/adapter/                       # main.go (entrypoint), health.go (/healthz /readyz /metrics)
providers/efi/
  adapter/
    spec.go                        # ← AUTHORITATIVE: Describe()/contract, AdapterVersion, operation consts
    spec_test.go                   # asserts the Describe() contract
    adapter.go                     # Execute() dispatcher (legacy switch) + EfiClient plumbing
    mtls.go                        # LoadTLSConfig(cfg) — EFI-prefixed mTLS via SDK mtls pkg
    metrics.go                     # Prometheus collectors (AdapterUp, etc.)
    reconcile.go                   # WireReconcilers — SDK reconcile.Dispatch table (§6.5 emission)
    webhook_server.go              # inbound EFI Pix webhook listener (mTLS)
    legacy_aliases_test.go         # v1.x → v2.0.0 operation-name alias coverage
    capabilities/                  # one file+test per capability (ensure_charge, refund_charge, ...)
    reactor/efi_webhook_received.go # inbound-callback reactor + EmitFunc
    testdata/                      # test.p12 / test.der mTLS fixtures
  config/config.go                 # EFI_* env knobs → typed Config
  efiapi/client.go                 # EfiClient: OAuth client-credentials + mTLS transport to EFI
  message/                         # describe.go, execute.go (hybrid bridge), rpc.go — SDK RPC handlers
family/                            # contract/types.go (AdapterDescribeResponse etc.), manifest.json
pkg/contractcheck/                 # describe-vs-execute drift lint (exercised by go test)
manifest/                         # REGISTERED manifest — integration_type.json, capabilities/*.yaml, instance example
docs/                              # operator + dev docs
yggdrasil-quickstart.yaml          # `yggdrasil install` bundle (kind=integration_quickstart)
Dockerfile, docker-compose*.yml, Taskfile.yml, .env.example
vendor/                            # vendored deps (go.mod uses vendoring)
```

## Stack

- Go 1.25, vendored deps.
- `github.com/dakasa-yggdrasil/yggdrasil-sdk-go v0.8.3` — the adapter
  uses the SDK `adapter`, `rpc`, `mtls`, and `sdk/reconcile` packages.
  This is NOT a from-scratch RPC layer; describe/execute go through the
  SDK adapter (`adapter.New(...)` in `cmd/adapter/main.go`).
- `go.uber.org/zap` (logging), `prometheus/client_golang` (metrics),
  OpenTelemetry OTLP gRPC tracing (no-op unless
  `OTEL_EXPORTER_OTLP_ENDPOINT` set).

## Transport

Selected at runtime by `YGGDRASIL_TRANSPORT` (see `Describe()` in
`spec.go` and the `switch` in `cmd/adapter/main.go`):

- **`http_json` (default)** — HTTP listener on `ADAPTER_PORT` (default
  `8081`); endpoints `/rpc/describe` + `/rpc/execute`.
- **`amqp` / `rabbitmq`** — AMQP listener on `BROKER_URL` (fatal if
  unset); queues `yggdrasil.adapter.efi.describe` /
  `yggdrasil.adapter.efi.execute`.

A separate health server runs on `HEALTHCHECK_PORT` (default `8080`,
`/healthz` `/readyz` `/metrics`), and the inbound EFI webhook listener
on `EFI_WEBHOOK_PORT` (default `9079`, mTLS when a cert is loaded).

## AdapterVersion

**`2.4.0`** — the single source of truth is
`adapter.AdapterVersion` in `providers/efi/adapter/spec.go`. It is
wire-advertised in `Describe()` and version-checked in the describe
handshake (`providers/efi/message/describe.go`).

> `manifest/integration_type.json` (the *registered* manifest, not an
> example) is synced to `spec.go` at `spec.adapter.version` = **`2.4.0`**.
> No describe-dump tool exists in this repo, so when `AdapterVersion`
> bumps, hand-edit that field in the same change and re-run the
> contractcheck/spec tests (`go test ./...`).

## mTLS

EFI requires bidirectional mTLS for Pix calls. The cert is loaded from
**EFI-prefixed env** via the SDK `mtls` package, wrapped by
`LoadTLSConfig(cfg)` in `providers/efi/adapter/mtls.go`:

- `EFI_MTLS_ENABLED` (default `true`; `false` → mock/test mode, no cert).
- `EFI_CERTIFICATE` (path to P12) **or** `EFI_CERTIFICATE_BASE64`
  (base64 P12 bytes) — path wins when both set.

The resulting `*tls.Config` feeds both the outbound `EfiClient`
transport (`providers/efi/efiapi/client.go`) and the inbound webhook
server. The SDK also ships `mtls.LoadFromEnv("EFI")` with the same
prefix convention; this repo uses its own `LoadTLSConfig` wrapper
(driven by the typed `config.Config`) rather than calling `LoadFromEnv`
directly.

## Capabilities (canonical, from `spec.go`)

Resource types and their actions — see `ActionCatalog` / `ResourceTypes`
in `spec.go` for the authoritative descriptions and the
`manifest/capabilities/*.yaml` for the published per-capability specs:

- **charge**: `ensure_charge`, `ensure_due_charge`, `observe_charges`,
  `destroy_charge`.
- **pix_transaction**: `refund_charge`, `create_payout`,
  `handle_chargeback`.
- **webhook_subscription**: `ensure_webhook_subscription`,
  `observe_webhook_subscriptions`, `destroy_webhook_subscription`,
  `verify_webhook_signature`, `efi_webhook_received` (reactor).

Two SDK-only reconcile ops (`observe_due_charges`,
`destroy_due_charge`) exist ONLY in the `sdk/reconcile` dispatch table
— not in `SupportedExecuteOperations`. Legacy v1.x names
(`create_charge`, `get_statement`, …) still resolve via
`LegacyOperationAliases` (WARN-shimmed; removed in v3.0.0).

### Execute = hybrid bridge

`providers/efi/message/execute.go` routes inbound envelopes through
`reconcile.Dispatch` FIRST (activating §6.5 mutation-event emission for
charge / due_charge / webhook_subscription), then falls back to the
legacy `adapter.Execute` switch for action helpers and the reactor.
Reconcilers are installed by `ad.WireReconcilers(a, instanceID)` in
`main()` before `Register`.

## Non-negotiable rules

- **Keep the adapter standalone.** Do not import runtime/domain code
  from `yggdrasil-core` or the monorepo. The local contract types live
  in `family/contract/`; the only external Yggdrasil dependency is the
  public `yggdrasil-sdk-go`.
- **`describe` MUST stay aligned with `execute`.** `pkg/contractcheck`
  catches drift; do not silence it. When you add/rename a capability,
  update `spec.go`, the capability impl+test, `manifest/`, and the
  `docs/` in the same change.
- **Capability naming follows the `ensure_/observe_/destroy_` prefix
  convention** (plus `verify_*` and the `*_received` reactor here).
  Avoid `create_*`/`list_*`/`delete_*` for resource operations.
- **Fail fast over silent degradation.** No swallowing transport errors.
- **Business authority stays in `yggdrasil-core`.** This worker owns
  EFI integration runtime behavior only. It does NOT publish to AMQP
  itself — webhook events are emitted via a `publish_message` workflow
  run routed through `integration-rabbitmq-runtime` (see
  `newProductionEmitFunc` in `main.go`).

## Validation

```bash
go test ./...      # unit + spec contract + contractcheck lint
task test          # same, via Taskfile
task config        # render the compose config (sanity)
task build:image   # docker build -t integration-efi:local
task up / task down / task logs   # local stack via docker compose
```

## CI / image flow

`.github/workflows/`:

- `ci.yml` — go build/test (includes spec + contractcheck).
- `release.yml` — publishes the worker image to
  `ghcr.io/dakasa-yggdrasil/integration-efi`.
- `emit-deploy-event.yml` — POSTs the deploy event into yggdrasil-core.
