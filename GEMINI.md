# GEMINI

`integration-efi` is a standalone Yggdrasil **integration adapter** for
the EFI (formerly Gerencianet) **Brazilian Pix** payments provider:
charges, payouts, refunds, webhook subscriptions, and inbound Pix
webhook callbacks, over the Banco Central PIX API with OAuth
client-credentials + **mTLS**.

- `integration_type` / provider: `efi` (single-provider). Namespace:
  `global`. Domain: `payments`.
- **Authoritative contract: `Describe()` in
  `providers/efi/adapter/spec.go`** (AdapterVersion `2.4.0`,
  capabilities, schemas, resource types, transport). Trust it over any
  prose. Read `CLAUDE.md` for the full repo map and `AGENTS.md` for the
  rules-of-engagement summary.

## Focus areas

- Keep this repository transport/runtime focused; do not import
  `yggdrasil-core`/monorepo code. Local contract types live in
  `family/contract/`; the only external Yggdrasil dep is
  `yggdrasil-sdk-go`.
- `describe` must stay aligned with `execute` — `pkg/contractcheck`
  catches drift. Capability changes must update `spec.go`, the
  capability impl+test, `manifest/`, and `docs/` together.
- Capability naming uses `ensure_/observe_/destroy_` prefixes (plus
  `verify_*` and the `*_received` reactor). Avoid
  `create_*`/`list_*`/`delete_*` for resource operations.

## Runtime

- Transport via `YGGDRASIL_TRANSPORT`: `http_json` (default,
  `/rpc/describe` + `/rpc/execute`) or `amqp`. Health on
  `HEALTHCHECK_PORT` (8080); inbound webhook on `EFI_WEBHOOK_PORT`
  (9079). mTLS from EFI-prefixed env via `LoadTLSConfig`
  (`providers/efi/adapter/mtls.go`).
- Validate with `go test ./...` (or `task test`).

> `manifest/integration_type.json` pins `adapter.version` 2.2.0 — stale
> vs `spec.go`'s 2.4.0. The registered manifest is bumped deliberately,
> not as a doc edit.
