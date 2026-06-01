# AGENTS

## Repo role

`integration-efi` is a standalone Yggdrasil **integration adapter** for
the EFI (formerly Gerencianet) **Brazilian Pix** payments provider. It
exposes an honest adapter contract through `describe` and executes
capabilities (charges, payouts, refunds, webhook subscriptions, inbound
Pix callbacks) through `execute`, talking to the Banco Central PIX API
over OAuth client-credentials + **mTLS**.

- `integration_type` / provider: `efi` (single-provider, type ==
  provider). Manifest `namespace`: `global`. Domain: `payments`.
- **The authoritative contract is `Describe()` in
  `providers/efi/adapter/spec.go`** — AdapterVersion (`2.4.0`),
  capabilities, schemas, resource types, and transport all live there
  and are asserted by `spec_test.go` + the `pkg/contractcheck` lint.
  Trust `spec.go` over any prose. See `CLAUDE.md` for the full repo map.

## Non-negotiable rules

- Keep the adapter standalone. Do not import runtime/domain code from
  `yggdrasil-core` or the monorepo. The only external Yggdrasil
  dependency is the public `yggdrasil-sdk-go`; local contract types live
  in `family/contract/`.
- `describe` must stay aligned with what `execute` accepts.
  `pkg/contractcheck` catches drift — do not silence it.
- If you add/rename a capability, update `spec.go`, the capability
  impl+test under `providers/efi/adapter/capabilities/`, `manifest/`,
  and the `docs/` in the same change.
- Capability naming uses the `ensure_/observe_/destroy_` prefix
  convention (plus `verify_*` and the `*_received` reactor). Avoid
  `create_*`/`list_*`/`delete_*` for resource operations.
- Prefer failing fast over silently degrading. This worker owns EFI
  runtime behavior only; business authority stays in `yggdrasil-core`.

## Runtime

- Transport is selected by `YGGDRASIL_TRANSPORT`: `http_json` (default,
  endpoints `/rpc/describe` + `/rpc/execute` on `ADAPTER_PORT` 8081) or
  `amqp`/`rabbitmq` (queues on `BROKER_URL`).
- Health server on `HEALTHCHECK_PORT` (8080): `/healthz` `/readyz`
  `/metrics`. Inbound EFI webhook listener on `EFI_WEBHOOK_PORT` (9079),
  mTLS when a cert is loaded.
- mTLS loads from EFI-prefixed env (`EFI_MTLS_ENABLED`,
  `EFI_CERTIFICATE` / `EFI_CERTIFICATE_BASE64`) via `LoadTLSConfig` in
  `providers/efi/adapter/mtls.go`.
- Graceful shutdown on `SIGINT`/`SIGTERM`.

> Note: `manifest/integration_type.json` pins `adapter.version` 2.2.0,
> stale vs `spec.go`'s 2.4.0. `manifest/` is the registered manifest —
> bump it deliberately in a manifest release, not as a doc edit.

## Commands

```
go test ./...      # or: task test
task config
task build:image
task up / task down / task logs
```
