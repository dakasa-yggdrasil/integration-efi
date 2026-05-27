# Changelog

All notable changes to integration-efi will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased — 2026-05-27

### Changed

- Bump yggdrasil-sdk-go v0.6.0 → v0.7.0 to pick up the public
  `reconcile.Dispatch` API. The bump is API-compatible; existing
  call sites keep building.

### Notes — production migration to reconcile.Dispatch DEFERRED

- This adapter still routes execute through the hand-written
  `providers/efi/adapter.Execute` switch. Unlike the sibling
  stripe/nfeio adapters, integration-efi never wired
  `RegisterReconciler` (no per-resource `Reconciler[D,O]` impls)
  — the v2.0.0 cycle landed the canonical capability names + the
  legacy alias shim in `LegacyOperationAliases`, but skipped the
  Reconciler binding step.
- Migrating to `reconcile.Dispatch` requires first authoring
  Reconcilers for the 4 managed resources (charge, due_charge,
  webhook_subscription, refund) and a `WireReconcilers` function.
  That work is a separate cycle from this SDK pin bump.
- §6.5 mutation event auto-emission stays DORMANT for EFI until
  the Reconciler wiring lands.

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
