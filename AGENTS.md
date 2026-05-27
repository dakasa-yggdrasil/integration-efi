# AGENTS

## Repo role

This repository is a standalone Yggdrasil integration worker for EFI/Pix
(Brazilian Banco Central PIX API + EFI overlay). It exposes an honest
adapter contract through `describe` and executes 11 capabilities through
`execute`, plus an inbound webhook HTTP listener on `:9079` for EFI Pix
callbacks.

## Non-negotiable rules

1. **Keep the adapter standalone.** Do not import runtime/domain code from
   `yggdrasil-core` `internal/` packages. Use only `yggdrasil-sdk-go`
   public packages.
2. **`Describe()` must stay aligned with what `Execute()` accepts.**
   `pkg/contractcheck` enforces this in CI; do NOT silence it. Every
   capability added to `SupportedExecuteOperations` must also land in
   `ActionCatalog` + `ResourceTypes`.
3. **mTLS cert rotation runbook.** The P12 is stored in AWS Secrets
   Manager `dakasa/prod/efi-mtls`. Rotate via the Yggdrasil
   `secret-rotate` workflow; the adapter reads on every pod start (no
   in-process refresh).
4. **Webhook signature verification is mandatory.** Never accept inbound
   `/efi/webhook/pix` callbacks without `verify_webhook_signature`
   succeeding (the WebhookServer enforces `tls.RequireAndVerifyClientCert`
   when an mTLS config is loaded).
5. **Failing fast over silent degradation.** OAuth failures, mTLS
   handshake errors, missing cert sources MUST surface as adapter
   `Fatal()` exits, not best-effort fallbacks.
6. **Business authority stays in `yggdrasil-core`.** This worker owns
   only the EFI HTTP transport + webhook listener. Idempotency,
   workflow orchestration, retries, and downstream dedup are core
   concerns.

## Runtime expectations

- `/healthz` is liveness only (always 200).
- `/readyz` reflects RPC transport state.
- `/metrics` is the Prometheus scrape endpoint.
- mTLS-enforced webhook listener on `:9079` (configurable via
  `EFI_WEBHOOK_PORT`).
- Adapter RPC on `:8081` (configurable via `ADAPTER_PORT`) over HTTP
  (default) or AMQP (set `YGGDRASIL_TRANSPORT=amqp`).
- Graceful shutdown on `SIGINT`/`SIGTERM`.

## Commands

- `go test ./...`
- `go test -tags integration ./integration_tests/...` (skipped unless
  `RUN_INTEGRATION_TESTS=true`)
- `task config`
- `task build:image`
- `task up`
- `task down`

## Change checklist

- TDD: failing test → implementation → passing test → commit.
- Add the new capability to `SupportedExecuteOperations`,
  `ActionCatalog`, `ResourceTypes`, AND `manifest/capabilities/<op>.yaml`
  in the same change.
- Update `CLAUDE.md` capability table if you add or rename a capability.
- Never use `git commit --no-verify` or `git push --force`.
- Commit format: `<gitmoji> <short imperative description>` (English).
  Never co-author AI.
