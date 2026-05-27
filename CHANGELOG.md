# Changelog

All notable changes to integration-efi will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.3.1] — 2026-05-27

### Fixed

- **K8s Service rpc port missing** — added explicit
  `deploy/service.yaml` declaring two named ports (8080 health, 8081
  rpc). Pre-2.3.1 the live Service only exposed 8080, so
  yggdrasil-core forward-drift auto-sync hit "connection refused"
  reaching `/rpc/describe` via service DNS. The Phase C agent worked
  around this with `kubectl exec deploy/yggdrasil -- wget` against
  the pod IP — not a fix. Apply the manifest via `apply_manifest`
  workflow on next deploy. Container ports 8080 + 8081 are already
  what `cmd/adapter/health.go` and `cmd/adapter/main.go` bind to;
  this manifest aligns the Service routing.

### Changed

- **Bumped `yggdrasil-sdk-go` v0.8.0 → v0.8.1**. SDK patch closes
  the destroy resource_id inference gap for §6.5 mutation events.
  All `efi.<resource>.destroyed` events emit with the correct
  identifier on the wire (was silently rejected with HTTP 400 by
  yggdrasil-core before v0.8.1).

## [2.3.0] — 2026-05-27

### Changed

- **Bumped `yggdrasil-sdk-go` v0.7.0 → v0.8.0**. SDK ships an opt-in
  `DestroyWithDesired[D]` interface that lets reconcilers see the
  FULL desired payload during destruction — closing the latent
  destroy-credential bug at the SDK level per
  INTEGRATION_CONTRACT.md §5.b.
- **`chargeReconciler`, `dueChargeReconciler`,
  `webhookSubscriptionReconciler` implement `DestroyWithDesired`**.
  Each method merges the inbound ref into the desired payload the
  SDK forwards (so `txid` / `chave` is present for handlers), then
  routes through the same `dispatch()` helper Ensure / Observe use.
  Reserved bridge keys (`_integration` / `instance_id`) propagate to
  the destroy handler — mTLS cert path + EFI client_id reach
  clientForInstance the same way ensure_* / observe_* see them.

### Tests

- New `TestSDKDispatch_DestroyCharge_PreservesReservedKeys` exercises
  destroy_charge through reconcile.Dispatch end-to-end with the
  bridge-stashed `_integration` blob — proves the v0.8.0 +
  DestroyWithDesired combination propagates credentials to the
  destroy handler.

## [Unreleased] — 2026-05-27 (operator wiring, no code change)

### Operator changes

- Wired `YGGDRASIL_CORE_BASE_URL` and `YGGDRASIL_WORKFLOW_RUN_TOKEN`
  repository secrets via the new `integration-github` v2.4.0
  `ensure_repository_secret` capability. The `emit-deploy-event.yml`
  workflow's soft-skip path (commit `5059770`) is no longer reached —
  `emit-deploy-event` now actively POSTs to `yggdrasil-core` on every
  push to `main`. Verified via run #26522330088 (HTTP 201 from
  `${YGGDRASIL_CORE_BASE_URL}/api/v1/workflow-runs`). Operator note:
  the secret values were never logged; provisioning was idempotent
  via the libsodium sealed-box GET-public-key → PUT-encrypted path
  inside integration-github.

## [2.2.0] — 2026-05-27

### Added

- Production runtime migrated to `sdk/reconcile.Dispatch` via the
  Option B hybrid bridge (matches the integration-slack /
  integration-stripe v2.x pattern). `controllers/message`
  `ExecuteHandler` now routes inbound envelopes through the SDK
  reconcile dispatch table FIRST, falling back to the legacy
  `adapter.Execute` switch when the operation has no registered
  Reconciler. The legacy fallback preserves the existing semantics
  for action helpers + reactor replay paths.
- 3 `Reconciler[reconcilePayload, reconcilePayload]` impls authored
  in `providers/efi/adapter/reconcile.go`:
  - `chargeReconciler` — `ensure_charge` / `observe_charges` /
    `destroy_charge`. Routes through the existing capability
    handlers (POST /v2/cob or PUT /v2/cob/{txid}, GET, PATCH).
  - `dueChargeReconciler` — `ensure_due_charge` (PUT /v2/cobv/{txid})
    plus SDK-only `observe_due_charges` / `destroy_due_charge`. The
    last two reuse the immediate-charge handlers (BCB Pix exposes
    cob and cobv records under the same /v2/cob/{txid} GET + PATCH
    paths).
  - `webhookSubscriptionReconciler` — `ensure_webhook_subscription` /
    `observe_webhook_subscriptions` / `destroy_webhook_subscription`.
    Routes through the EFI v2 webhook surface with the existing v3
    fallback for accounts that only accept /v3/gn/webhook/{chave}.
- `WireReconcilers(a, instanceID)` function: installs the SDK
  reconcile handlers BEFORE the adapter `.Register(...)` chain so the
  hybrid bridge can take over the execute capability while preserving
  the dispatch table the SDK auto-installs. Wired in `cmd/adapter/main.go`
  ahead of register.
- §6.5 mutation event auto-emission now lives for the resource-typed
  ops (`charge`, `due_charge`, `webhook_subscription`) when
  `YGGDRASIL_CORE_URL` + `YGGDRASIL_RUN_TOKEN` are set. Empty env →
  `events.NoopEmitter` so dev / unit tests stay deterministic. Emission
  is best-effort: failures log WARN but do NOT fail the capability
  call (per `reconcile.WithEmitter` docstring).
- Two SDK-only canonical operations (`observe_due_charges`,
  `destroy_due_charge`) admitted by `SupportsExecuteCapability` so the
  controllers/message gate doesn't reject them BEFORE
  `reconcile.Dispatch` can route. They are NOT in the legacy
  `SupportedExecuteOperations` slice (so the legacy `Execute` switch
  never sees them); the SDK dispatch table is their sole entry point.
- New unit test file `providers/efi/adapter/reconcile_test.go`:
  10 table-driven tests against fake dispatch table + an end-to-end
  test driving `reconcile.Dispatch` through all 3 Reconcilers (9
  canonical ops total) + a legacy alias WARN counter test.

### Action allowlist + helper + reactor stay in legacy switch path

- `refund_charge` (action — PUT /v2/pix/{e2eId}/devolucao/{id})
- `create_payout` (action — PUT /v3/gn/pix/{idEnvio})
- `handle_chargeback` (action — internal pass-through)
- `verify_webhook_signature` (helper — pure x509 DER parse)
- `efi_webhook_received` (reactor — framework-invoked)

These are NOT resource-typed `ensure_/observe_/destroy_` operations
and therefore stay in the `adapter.Execute` switch. The hybrid bridge
falls back to that path on `reconcile: unsupported operation`. §6.5
emission for the money-movement actions (`refund_charge`,
`create_payout`) is a separate cycle (would emit `efi.refund.created`
/ `efi.payout.created` per the contract's exemption for non-idempotent
allowlist actions).

## [2.1.0] — 2026-05-27

### Added

- Bump yggdrasil-sdk-go v0.5.0 → v0.6.0 to pick up the additive
  `sdk/events` package + the new `reconcile.WithEmitter` /
  `WithProvider` / `WithInstanceID` options. The bump is API-compatible
  (only new options were added); existing call sites keep building.

### Notes

- This adapter still routes execute through the hand-written
  `providers/efi/adapter` switch (no `sdk/reconcile.RegisterReconciler`
  call yet); emission of `efi.<resource>.ensured` /
  `efi.<resource>.destroyed` mutation events per INTEGRATION_CONTRACT.md
  §6.5 activates once the adapter migrates onto the SDK reconcile
  dispatch path. The SDK is now positioned (v0.6.0) so this can land
  as a follow-up commit without further SDK changes.
- Vendor directory refreshed via `go mod vendor` to pick up the new
  `sdk/events` package files.

## [2.0.0] — 2026-05-27

### BREAKING CHANGES — Yggdrasil universal capability naming convention

6 renames + 1 merge + 2 net-new ops aligned with the universal naming
convention (`ensure_/observe_/destroy_/discover_`). Pre-v2.0.0 names
continue to route through the v2.0.0 dispatch path with a WARN log;
removal target v3.0.0.

#### Renames

| v1.x                         | v2.0.0                            |
|------------------------------|------------------------------------|
| `create_charge`              | `ensure_charge`                    |
| `create_due_charge`          | `ensure_due_charge`                |
| `get_charge_status`          | `observe_charges` (filter txid)    |
| `get_statement`              | `observe_charges` (filter range)   |
| `register_webhook_endpoint`  | `ensure_webhook_subscription`      |
| `unregister_webhook_endpoint`| `destroy_webhook_subscription`     |

#### New

- `destroy_charge` (PATCH /v2/cob/{txid} status=REMOVIDA)
- `observe_webhook_subscriptions`
