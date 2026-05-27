# CLAUDE — integration-efi

Start with `AGENTS.md` for the non-negotiable rules. This file adds product
context that AI assistants need to make safe edits.

## What this repo is

Public Yggdrasil adapter for Brazilian Pix payments (Banco Central PIX API +
EFI overlay). Single-provider layout (no multi-provider matrix like
`integration-rabbitmq`).

Replaces:

- The EFI provider in `dakasa-co/integration-webhooks-external`.
- The in-process EFI client in `dakasa-app-fe/backend/dakasa-identities`
  (`client/efi-bank.go`).

Repo: `github.com/dakasa-yggdrasil/integration-efi` (open source, Apache 2.0).

## Stack

- Go 1.25.
- `github.com/dakasa-yggdrasil/yggdrasil-sdk-go` (v0.5.0) — adapter,
  rpc, mtls, webhookhttp, sig/hmac packages. v0.5.0 ships the
  `sdk/reconcile` package; this adapter mirrors the same compat-shim
  behavior (`LegacyOperationAliases`) in its local dispatch path.
- `golang.org/x/crypto/pkcs12` — decode the P12 mTLS bundle.
- `go.uber.org/zap` — structured logging.
- `github.com/prometheus/client_golang` — metrics on `/metrics`.
- `go.opentelemetry.io/otel` — spans on every outbound EFI call.

## Repo layout

```
cmd/adapter/                   # main.go (SDK bootstrap) + health.go (/healthz, /readyz, /metrics)
providers/efi/
  adapter/                     # spec.go (Describe), adapter.go (Execute switch),
                               # client.go (HTTP+OAuth), mtls.go, webhook_server.go, metrics.go
  adapter/capabilities/        # one file per capability (12 total in v2.0.0)
  adapter/reactor/             # efi_webhook_received reactor
  config/                      # env knobs loader
  message/                     # DescribeHandler + ExecuteHandler + rpc helpers
family/
  contract/                    # AdapterDescribeResponse etc. — same shape as integration-rabbitmq
  manifest.json                # integration_family manifest
pkg/contractcheck/             # vendored from integration-template@95335d7 — lint Describe alignment
manifest/
  capabilities/                # 12 YAMLs (one per capability) in v2.0.0
  integration_type.json
  integration_instance.example.json
integration_tests/             # //go:build integration; RUN_INTEGRATION_TESTS=true to run
.github/workflows/             # ci, release, emit-deploy-event
```

## Capabilities (12, v2.0.0)

| Capability                          | Idempotent | EFI endpoint                                       |
|-------------------------------------|------------|----------------------------------------------------|
| `ensure_charge`                     | yes        | POST /v2/cob or PUT /v2/cob/{txid}                 |
| `ensure_due_charge`                 | yes        | PUT /v2/cobv/{txid}                                |
| `observe_charges`                   | yes        | GET /v2/cob/{txid} OR GET /v2/cob?inicio=&fim=     |
| `destroy_charge`                    | yes        | PATCH /v2/cob/{txid} status=REMOVIDA (404=success) |
| `refund_charge`                     | via id     | PUT /v2/pix/{e2eId}/devolucao/{id}                 |
| `create_payout`                     | via idEnvio| PUT /v3/gn/pix/{idEnvio} (IntermediateIrreversible)|
| `handle_chargeback`                 | yes        | (internal — pass-through)                          |
| `ensure_webhook_subscription`       | yes        | PUT /v2/webhook/{chave} (v3 fallback)              |
| `observe_webhook_subscriptions`     | yes        | GET /v2/webhook OR GET /v2/webhook/{chave}         |
| `destroy_webhook_subscription`      | yes        | DELETE /v2/webhook/{chave} (404=success)           |
| `verify_webhook_signature`          | yes        | (internal — x509 DER parse)                        |
| `efi_webhook_received` (reactor)    | yes        | (inbound :9079 → identities.efi.pix-receive.q)     |

### v2.0.0 rename map (v1.x → v2.0.0)

| v1.x (deprecated)              | v2.0.0 (canonical)              | Notes                                     |
|--------------------------------|----------------------------------|-------------------------------------------|
| `create_charge`                | `ensure_charge`                  | Same semantics, prefix realigned          |
| `create_due_charge`            | `ensure_due_charge`              | Same semantics, prefix realigned          |
| `get_charge_status`            | `observe_charges` (filter txid)  | Merged into single observe op             |
| `get_statement`                | `observe_charges` (filter range) | Merged into single observe op             |
| `register_webhook_endpoint`    | `ensure_webhook_subscription`    | Resource type webhook → webhook_subscription |
| `unregister_webhook_endpoint`  | `destroy_webhook_subscription`   | Same                                      |

v1.x names continue to route through the v2.0.0 dispatch path with a
WARN log (`LegacyOperationAliases` in `providers/efi/adapter/spec.go`).
The shim is removed in v3.0.0 (matches SDK v0.6.0 cadence).

## Cutover plan

See spec `docs/superpowers/specs/2026-05-26-integration-efi-design.md` Section 6.

## Recent significant work

- 2026-05-27 — v2.0.0 universal capability convention rollout: 6 renames +
  1 merge + 2 net-new ops (`observe_webhook_subscriptions`, `destroy_charge`).
  SDK bumped to v0.5.0. v1.x compat shim in `LegacyOperationAliases` keeps
  callers working through the v2.x deprecation window (removed in v3.0.0).
- 2026-05-26 — initial v1.0.0 scaffold + 11 capabilities + reactor + webhook
  listener + Prometheus + OTel.

## Honest gotchas

- **mTLS cert rotation**: P12 stored in AWS SM `dakasa/prod/efi-mtls`. The
  adapter loads from `EFI_CERTIFICATE` (file path) at startup; rotation
  requires pod restart via the Yggdrasil secret-rotate workflow.
- **ECR Pull Through Cache delay**: images push to GHCR; ECR PTC cache lag
  can be up to 12h. Trigger `ecr-pull-fresh` Yggdrasil workflow to force
  refresh.
- **Webhook dedup is downstream**: the adapter does NOT dedup. The
  identities consumer enforces `webhook_event_efi.e2e_id` UNIQUE.
- **`/v3/gn/webhook/{chave}` fallback**: some EFI accounts only accept the
  v3 path. `ensure_webhook_subscription` tries v2 first then falls back to
  v3 on 404 (mirrors legacy `client/efi-bank.go:383-391`).
