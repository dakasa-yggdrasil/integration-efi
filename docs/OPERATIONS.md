# Operations — integration-efi

Health endpoints, Prometheus metrics, webhook ownership, and common failures,
all grepped from the adapter source (`cmd/adapter/health.go`,
`providers/efi/adapter/metrics.go`, `providers/efi/efiapi/metrics.go`).

← Back to the [README](../README.md) · part of
[Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core).

---

## Health & readiness

The health server listens on `HEALTHCHECK_PORT` (default **`8080`**),
independent of the RPC transport.

| Endpoint | Method | Behavior |
|---|---|---|
| `/healthz` | GET | Liveness — always `200 ok`. |
| `/readyz` | GET | Returns `200 ready`. (Liveness-style; not gated on broker state in this adapter.) |
| `/metrics` | GET | Prometheus exposition. |

Kubernetes `Service` (`deploy/service.yaml`) maps:

- port `8080` (named `health`) → liveness/readiness probes + Prometheus scrape
- port `8081` (named `rpc`) → `/rpc/describe` + `/rpc/execute` (HTTP-JSON)

> The RPC port `8081` MUST be routed by the Service or yggdrasil-core's
> forward-drift / auto-sync fails with `connection refused` when describing the
> adapter via Service DNS (`integration-efi.dakasa.svc`). This was the 2.3.1
> fix — pre-2.3.1 the live Service only exposed `8080`.

## Metrics

Two metric families: adapter-level (`providers/efi/adapter/metrics.go`) and
HTTP-client-level (`providers/efi/efiapi/metrics.go`).

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `efi_adapter_up` | gauge | — | `1` when the adapter is healthy (set at boot). |
| `efi_request_duration_seconds` | histogram | `op`, `status_class` | Duration of outbound EFI API calls. |
| `efi_request_errors_total` | counter | `op`, `status` | Non-2xx EFI responses. |
| `efi_oauth_token_refreshes_total` | counter | `result` | OAuth token refreshes (`success` / `decode_fail` / `empty_token`). |
| `efi_mtls_handshake_failures_total` | counter | — | Outbound mTLS handshake failures. |

`op` label values (`classifyPath`): `oauth`, `cob`, `create_due_charge`,
`get_statement`, `refund_charge`, `create_payout`, `webhook`,
`automatic_webhook` (`/v2/webhookrec` and `/v2/webhookcobr`), `other`.
`status_class` ∈ `2xx` / `4xx` / `5xx` / `transport`.

Each outbound EFI call also opens **one OpenTelemetry span** (`efi.<op>`) when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set.

### Useful queries

```promql
# Adapter healthy?
efi_adapter_up

# Outbound error rate by operation
rate(efi_request_errors_total[5m])

# p99 latency for immediate charges
histogram_quantile(0.99, rate(efi_request_duration_seconds_bucket{op="cob"}[5m]))
```

`efi_webhook_received_total` was removed in 2.5.1 with the webhook listener.
Drop any dashboard panel or alert that still reads it.

---

## Webhooks

The adapter runs **no inbound webhook listener**. It manages EFI webhook
registrations (`ensure_webhook_subscription`, `observe_webhook_subscriptions`,
`destroy_webhook_subscription`); EFI then delivers Pix callbacks to the
registered `webhook_url` plus `/pix`, which another service serves. For DaKasa
production see [USAGE.md](USAGE.md#6-receiving-inbound-pix-callbacks).

```mermaid
flowchart LR
  wf["Yggdrasil workflow"] -- "ensure_webhook_subscription" --> efi["integration-efi"]
  efi -- "PUT /v2/webhook/{chave}" --> bcb["EFI / BCB"]
  bcb -- "POST webhook_url + /pix (mTLS)" --> rx["service behind webhook_url"]
```

### What 2.5.1 removed

Up to 2.5.0 the adapter listened on `EFI_WEBHOOK_PORT` (`9079`,
`POST /efi/webhook/pix`) and handed each callback to the `efi_webhook_received`
reactor, which posted a `global/publish-message` workflow run to
`${YGGDRASIL_CORE_BASE_URL}/api/v1/workflow-runs` with
`YGGDRASIL_WORKFLOW_RUN_TOKEN`. That workflow and its `global/rabbitmq-runtime`
instance were never registered, and no Service or ingress routed the port, so
the path delivered nothing. The listener, the dispatch, both env vars,
`EFI_WEBHOOK_PORT` and `efi_webhook_received_total` are gone.

`efi_webhook_received` stays in the contract with no event sink. Through
Execute, a non-empty `pix` array fails with `ErrNoReactorSink`; an empty one
returns `{ emitted: false }`.

---

## Common failures

| Symptom | Likely cause | Fix |
|---|---|---|
| Worker exits at boot with `EFI_MTLS_ENABLED=true but no EFI_CERTIFICATE or EFI_CERTIFICATE_BASE64 set` | mTLS on, no cert source | Mount a P12 at `EFI_CERTIFICATE` or set `efi_certificate_base64`. |
| `oauth_failed` error / `efi_oauth_token_refreshes_total{result="decode_fail"}` | Bad `efi_client_key_id`/`efi_client_secret` | Verify credentials; secret rotated? |
| `efi_mtls_handshake_failures_total` climbing | Expired/wrong client cert | Rotate the P12; restart the pod. |
| `efi_request_errors_total{status="429"}` | EFI rate limit | Back off; transient (`IsTransient()` retries 429/503/504). |
| `connection refused` describing via Service DNS | Service missing rpc port `8081` | Apply `deploy/service.yaml` (the 2.3.1 fix). |
| `efi_webhook_received` fails with `no event sink is wired` | The reactor has had no sink since 2.5.1 | Expected. Pix callbacks belong to the service behind `webhook_url`, not this adapter. |

### Transient vs terminal (outbound)

`efiapi.IsTransient()` marks **429, 503, 504** as retryable; **500** is
deliberately non-transient; all other 4xx are terminal (caller bug). The SDK
handles retry of transient errors; transport-level errors are the SDK's
responsibility.

---

## Staging validation runbook

Before a production cutover, run the staged validation procedure (100 charges;
the webhook callback tests retired with the listener in 2.5.1) in:

→ **[RUNBOOK_STAGING_VALIDATION.md](RUNBOOK_STAGING_VALIDATION.md)**

The acceptance gate: the charge test passes and zero alerts fire in a 30-minute
observation window.

## Graceful shutdown

On `SIGINT`/`SIGTERM` the adapter (`cmd/adapter/main.go`) cancels the run
context, stops the health server (10s deadline), and drains the SDK adapter.
mTLS, OTel, and the RPC listener all shut down cleanly.
</content>
