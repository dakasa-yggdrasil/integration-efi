# Configuration — integration-efi

Every field below is derived from source: `manifest/integration_type.json`,
`providers/efi/adapter/spec.go` (the `Describe()` contract), and
`providers/efi/config/config.go` (the env loader).

← Back to the [README](../README.md) · part of
[Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core).

Configuration has three layers:

1. **Credentials** — declared on the integration instance, marked `secret` where
   sensitive. Yggdrasil resolves them from your secret store via `credentials`.
2. **Instance config** — per-instance, non-secret behavioral settings with
   defaults.
3. **Runtime env vars** — read by the worker process at boot
   (`config.Load()` + `cmd/adapter/main.go`).

---

## 1. Credential schema

`credential_schema.mode = "inline"`. Required: `efi_client_key_id`,
`efi_client_secret`.

| Field | Type | Required | Secret | Description |
|---|---|:--:|:--:|---|
| `efi_client_key_id` | string | **yes** | — | EFI Pix API client key ID. Used as the OAuth Basic-auth username. |
| `efi_client_secret` | string | **yes** | **yes** | EFI Pix API client secret. OAuth Basic-auth password. |
| `efi_certificate_base64` | string | no | **yes** | Base64-encoded P12 mTLS certificate bytes. Alternative to mounting an `EFI_CERTIFICATE` file. Required (one cert source) when `mtls_enabled = true`. Depends on `mtls_enabled`. |

> **mTLS is mandatory for EFI Pix.** When `mtls_enabled = true` (the default),
> the adapter must have a P12 cert from **either** `efi_certificate_base64`
> **or** a mounted file at `EFI_CERTIFICATE`. If neither is set, the worker
> fails fast at boot with
> `EFI_MTLS_ENABLED=true but no EFI_CERTIFICATE or EFI_CERTIFICATE_BASE64 set`
> (`providers/efi/adapter/mtls.go`).

Example instance `credentials` block (`manifest/integration_instance.example.json`)
— values are `credentials_ref` placeholders resolved from your secret store:

```json
"credentials": {
  "efi_client_key_id":      "<from-aws-sm:dakasa/prod/efi-mtls#client_key_id>",
  "efi_client_secret":      "<from-aws-sm:dakasa/prod/efi-mtls#client_secret>",
  "efi_certificate_base64": "<from-aws-sm:dakasa/prod/efi-mtls#p12_base64>"
}
```

> The `<from-aws-sm:...>` form is an example reference shape only — Yggdrasil is
> provider-agnostic; any configured secret backend works. The adapter never
> hardcodes a secret store.

---

## 2. Instance schema

`instance_schema.mode = "inline"`. All fields have defaults.

| Field | Type | Default | Description |
|---|---|---|---|
| `base_url` | string | `https://pix.api.efipay.com.br` | EFI Pix API base URL. Use `https://pix-h.api.efipay.com.br` for homologation. |
| `sandbox` | boolean | `false` | Whether this instance points at EFI homologation (`pix-h`). |
| `mtls_enabled` | boolean | `true` | Enforce mTLS for outbound + inbound. Disable only for mock/test instances. |
| `webhook_port` | integer | `9079` | Port the adapter listens on for inbound EFI webhook callbacks. |

Example instance `config` block:

```json
"config": {
  "base_url":     "https://pix.api.efipay.com.br",
  "sandbox":      false,
  "mtls_enabled": true,
  "webhook_port": 9079
}
```

---

## 3. Runtime environment variables

Read at process boot by `config.Load()` (`providers/efi/config/config.go`) and
`cmd/adapter/main.go`. `.env.example` ships sensible local defaults.

### EFI provider knobs

| Env var | Default | Maps to | Notes |
|---|---|---|---|
| `EFI_API_CLIENT_KEY_ID` | _(empty)_ | `ClientKeyID` | OAuth Basic-auth username. |
| `EFI_API_CLIENT_SECRET` | _(empty)_ | `ClientSecret` | **Secret.** OAuth Basic-auth password. |
| `EFI_CERTIFICATE` | _(empty)_ | `CertificatePath` | Path to a mounted P12 file (e.g. `/etc/efi/cert.p12`). Takes precedence over base64. |
| `EFI_CERTIFICATE_BASE64` | _(empty)_ | `CertificateBase64` | **Secret.** Base64 P12 bytes — used when no `EFI_CERTIFICATE` path is set. |
| `EFI_BASE_URL` | `https://pix.api.efipay.com.br` | `BaseURL` | EFI Pix API base URL. |
| `EFI_MTLS_ENABLED` | `true` | `MTLSEnabled` | `true/1/yes` enable, `false/0/no` disable. |
| `EFI_WEBHOOK_PORT` | `9079` | `WebhookPort` | Inbound webhook listener port. |

### Adapter / transport knobs

| Env var | Default | Notes |
|---|---|---|
| `YGGDRASIL_TRANSPORT` | `http` | `http`/`http_json` (default) or `amqp`/`rabbitmq`. |
| `ADAPTER_PORT` | `8081` | RPC HTTP listen port (`/rpc/describe`, `/rpc/execute`) when transport is HTTP. |
| `HEALTHCHECK_PORT` | `8080` | Health server (`/healthz`, `/readyz`, `/metrics`). |
| `BROKER_URL` | _(empty)_ | RabbitMQ URL. **Required and fatal-if-empty** only when `YGGDRASIL_TRANSPORT=amqp`. |
| `YGGDRASIL_INTEGRATION_INSTANCE_NAME` | _(empty)_ | Fallback instance ID used when an inbound envelope carries no `integration.instance.name`. Payload-bound values take precedence. |

### Event emission (webhook reactor → core)

The webhook reactor POSTs a `publish_message` workflow run back to
yggdrasil-core. Without these, the reactor logs a warning and skips emit (dev
mode).

| Env var | Default | Notes |
|---|---|---|
| `YGGDRASIL_CORE_BASE_URL` | _(empty)_ | Base URL of yggdrasil-core. Empty → emit skipped (dev mode). |
| `YGGDRASIL_WORKFLOW_RUN_TOKEN` | _(empty)_ | **Secret.** Bearer token for `POST /api/v1/workflow-runs`. |

> The §6.5 mutation-event auto-emission path (for `ensure_*`/`destroy_*` on
> resource types) reads the same core URL + token; the in-tree reconcile bridge
> falls back to a no-op emitter when they are unset, keeping dev/unit runs
> deterministic.

### Observability

| Env var | Default | Notes |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | _(empty)_ | OTLP gRPC endpoint. Empty → no-op tracer (spans are dropped). One span per outbound EFI call. |

### HTTP server timeouts (Compose-wired)

Surfaced via `docker-compose.yml` for the worker's HTTP servers; defaults shown.

| Env var | Default |
|---|---|
| `HTTP_READ_HEADER_TIMEOUT_SECONDS` | `10s` |
| `HTTP_READ_TIMEOUT_SECONDS` | `30s` |
| `HTTP_WRITE_TIMEOUT_SECONDS` | `30s` |
| `HTTP_IDLE_TIMEOUT_SECONDS` | `120s` |

---

## Ports

| Port | Purpose | Source |
|---|---|---|
| `8080` | Health + readiness + metrics (`/healthz`, `/readyz`, `/metrics`) | `cmd/adapter/health.go` |
| `8081` | RPC HTTP (`/rpc/describe`, `/rpc/execute`) | `cmd/adapter/main.go` |
| `9079` | Inbound EFI Pix webhook listener (`/efi/webhook/pix`, mTLS) | `providers/efi/adapter/webhook_server.go` |

The Kubernetes `Service` (`deploy/service.yaml`) exposes named ports `health`
(8080) and `rpc` (8081). Webhook port 9079 is deliberately **not** in the
Service — that ingress is routed separately behind the external webhook receiver
/ mTLS terminator.

---

## Version truth

The wire-advertised adapter version is the `AdapterVersion` constant in
`providers/efi/adapter/spec.go` — currently **`2.4.0`**. The `describe` handshake
returns it, and yggdrasil-core compares it against any `expected_version` the
caller passes.

Known stale catalog metadata (the binary is the source of truth):

| Source | Says | Reality |
|---|---|---|
| `providers/efi/adapter/spec.go` (`AdapterVersion`) | **`2.4.0`** | ✅ authoritative |
| `manifest/integration_type.json` (`adapter.version`) | `2.2.0` | stale |
| `yggdrasil-quickstart.yaml` (`image_tag`) | `v1.0.0` | stale |
| `CHANGELOG.md` (top entry) | `2.3.1` | trails the constant |
| git tags | `v1.0.0`, `v2.0.0` | trail the constant |

When pinning a deployment, prefer an immutable image tag published by the
`release` workflow (`sha-<short>`, or `vX.Y.Z` on a tag push) over the stale
`image_tag` in the quickstart bundle.
</content>
