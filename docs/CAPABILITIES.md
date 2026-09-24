# Capabilities — integration-efi

14 capabilities: **13 user-dispatched** (`category: capability`) + **1 reactor**
(`efi_webhook_received`, `category: reactor`, no event sink since 2.5.1). Each
section below is derived from `manifest/capabilities/*.yaml` and the
route/idempotency metadata in `providers/efi/adapter/spec.go`.

← Back to the [README](../README.md) · see also
[USAGE.md](USAGE.md) · [CONFIGURATION.md](CONFIGURATION.md).

Capabilities are grouped by `resource_type`. Legacy v1.x names still route (with
a deprecation WARN) until v3.0.0 — see [Legacy aliases](#legacy-aliases).

---

## Resource type: `charge`

Canonical prefix `thirdparty.efi.charge` · identity `charge.{txid}` ·
not discoverable.

### `ensure_charge`

Ensure an immediate Pix charge (`cob`) exists. `POST /v2/cob` (auto-generated
txid) or `PUT /v2/cob/{txid}` when caller-supplied. Idempotent by `txid` —
repeat PUTs reconcile to the same charge identity.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `valor` | object | yes | `{ original: string }` — amount, e.g. `"10.00"`. |
| `chave` | string | yes | The receiving Pix key. |
| `txid` | string | no | Caller-supplied transaction id (enables idempotent PUT). |
| `expiracao` | integer | no | Expiry seconds (default `3600`). |

**Output:** `txid`, `location`, `pixCopiaECola`, `status`, `created_at`.

### `ensure_due_charge`

Ensure a due-date Pix charge (`cobv`) exists. `PUT /v2/cobv/{txid}`. Idempotent —
caller-supplied `txid` is required (cobv is always upserted).

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `txid` | string | yes | Caller-supplied transaction id. |
| `valor` | object | yes | `{ original: string }`. |
| `chave` | string | yes | Receiving Pix key. |
| `calendario` | object | yes | `{ dataDeVencimento (req), validadeAposVencimento }`. |
| `devedor` | object | yes | `{ cpf (req), nome (req) }`. |
| `multa` / `juros` / `abatimento` / `desconto` | object | no | Penalty / interest / rebate / discount blocks. |

**Output:** `txid`, `status`, `location`, `pixCopiaECola`.

### `observe_charges`

Observe Pix charges. **Read-only.** Caller MUST supply either `{txid}` OR
`{inicio, fim}`. v2.0.0 merge of `get_charge_status` + `get_statement`.

- `{txid}` → single charge: `GET /v2/cob/{txid}`
- `{inicio, fim}` → paged statement window: `GET /v2/cob?inicio=&fim=&page=&page_size=&status=`

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `txid` | string | one-of | Single-charge variant. |
| `inicio` | string | one-of | RFC3339 start (statement window, with `fim`). |
| `fim` | string | one-of | RFC3339 end (statement window, with `inicio`). |
| `status` | string | no | Filter the statement by status (statement variant only). |
| `page` | integer | no | Default `0`. |
| `page_size` | integer | no | Default `100`. |

**Output (raw upstream):** single-charge variant `{ txid, status, pix[] }`;
statement variant `{ parametros, cobs[] }`.

### `destroy_charge`

Remove (cancel) a Pix charge by `txid`. `PATCH /v2/cob/{txid}` with
`status=REMOVIDA_PELO_USUARIO_RECEBEDOR` per BCB Pix spec. A 404 from EFI is
treated as already-absent success (idempotent).

| Input | Type | Required |
|---|---|:--:|
| `txid` | string | yes |

**Output:** `destroyed` (bool), `txid`, `already_absent` (bool).

---

## Resource type: `pix_transaction`

Canonical prefix `thirdparty.efi.pix` · identity `pix.{e2eId}` ·
not discoverable.

### `refund_charge`

Refund a Pix transaction. `PUT /v2/pix/{e2eId}/devolucao/{id}`. Idempotent — the
caller-supplied `id` is the dedup key.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `e2eId` | string | yes | End-to-end id of the original Pix. |
| `id` | string | yes | Refund id (dedup key). |
| `valor` | string | yes | Refund amount. |
| `natureza` | string | no | Refund nature. |
| `descricao` | string | no | Free-text description. |

**Output:** `id`, `rtrId`, `valor`, `status`, `horario` (object).

### `create_payout`

Send a Pix payout (`envio`). `PUT /v3/gn/pix/{idEnvio}`. **Classified
`IntermediateIrreversible` — money movement.** `idempotent: false` is a *safety*
classification: EFI provides server-side idempotency by `idEnvio`, but callers
MUST treat the operation as opaque/non-retryable. Rate-limit hint: `500/second`.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `idEnvio` | string | yes | Caller payout id (server idempotency key). |
| `valor` | string | yes | Amount. |
| `pagador` | object | yes | `{ chave (req) }` — the paying key. |
| `favorecido` | object | yes | `{ chave | cpf | contaBanco }` — the beneficiary. |

**Output:** `idEnvio`, `e2eId`, `valor`, `horario`, `status`.

### `handle_chargeback`

Acknowledge an EFI chargeback. **Pass-through — no HTTP call.** Idempotent by
`chargeback_id`.

| Input | Type | Required |
|---|---|:--:|
| `e2eId` | string | yes |
| `chargeback_id` | string | yes |
| `valor` | string | no |
| `status` | string | no |

**Output:** `e2eId`, `chargeback_id`, `status`, `processed` (bool).

> `refund_charge`, `create_payout`, and `handle_chargeback` are action/helper
> operations: they stay on the legacy `adapter.Execute` switch rather than the
> SDK reconcile path (they are not resource `ensure_/observe_/destroy_` ops).

---

## Resource type: `webhook_subscription`

Canonical prefix `thirdparty.efi.webhook_subscription` ·
identity `webhook_subscription.{chave}` · not discoverable.

### `ensure_webhook_subscription`

Ensure a Pix webhook subscription exists. `PUT /v2/webhook/{chave}` with a
`/v3/gn/webhook/{chave}` fallback on 404 (for accounts that only accept v3).
Idempotent — repeat calls reconcile URL/headers without creating duplicates.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `chave` | string | yes | The Pix key the webhook is attached to. |
| `webhook_url` | string | yes | HTTPS receiver **base** URL. Do not include `/pix`; EFI probes this URL during registration and appends `/pix` for real deliveries. DaKasa production uses `https://webhook.dakasa.me/efi/webhook`, which delivers to `/efi/webhook/pix`. |
| `skip_mtls_validation` | boolean | no | Sets EFI's `x-skip-mtls-checking` header (default `false`). |

**Output:** `ensured` (bool), `chave`, `endpoint`.

### `observe_webhook_subscriptions`

Observe Pix webhook subscriptions. **Read-only.** `{chave}` returns one
subscription (`GET /v2/webhook/{chave}`). Listing requires an explicit
`{inicio, fim}` window because EFI rejects an unbounded `GET /v2/webhook`.
Optional pagination uses `page`, `page_size`, or the `cursor` returned by the
previous page. The adapter maps them to the BCB query names.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `chave` | string | one-of | Single-subscription variant. |
| `inicio` | string | one-of | RFC3339 start of the creation window, with `fim`. |
| `fim` | string | one-of | RFC3339 end of the creation window, with `inicio`. |
| `page` | integer | no | Zero-based provider page, mutually exclusive with `cursor`. |
| `page_size` | integer | no | Items per page, from 1 through 1000. |
| `cursor` | string | no | Next-page cursor returned by the prior call, mutually exclusive with `page`. |

**Output (normalized upstream):** single-chave variant
`{ chave, webhookUrl, criacao }`; list variant
`{ parametros, webhooks: [...], cursor }`. Empty `cursor` means the last page.
Query values embedded in `webhookUrl` are redacted so HMAC-style URL secrets do
not enter workflow history, logs, or surfaces; query parameter names remain
visible for drift diagnosis.

### `destroy_webhook_subscription`

Remove a Pix webhook subscription. `DELETE /v2/webhook/{chave}`. A 404 is treated
as already-absent success (idempotent).

| Input | Type | Required |
|---|---|:--:|
| `chave` | string | yes |

**Output:** `destroyed` (bool), `chave`, `already_absent` (bool).

### `verify_webhook_signature`

Verify a peer x509 certificate from the inbound webhook handshake. **Pure
computation — no HTTP call.**

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `peer_cert_der` | string | yes | DER-encoded peer certificate. |
| `expected_issuer` | string | no | Issuer to assert against. |

**Output:** `valid` (bool), `subject`, `issuer`, `not_after`.

### `efi_webhook_received` — reactor

**Reactor, not user-dispatched; kept for contract compatibility.** Up to 2.5.0
the adapter's webhook listener invoked it and it emitted through a
`publish_message` workflow run that was never registered. 2.5.1 removed both,
so it now runs only through Execute, where `adapter.DefaultReactorEmit` has no
sink: it extracts the first `pix[]` entry, builds the normalized envelope below,
and fails with `ErrNoReactorSink` instead of emitting it.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `pix` | array | yes | Array of Pix objects; each `{ endToEndId, txid, valor, status, chave, horario, devolucoes[] }`. |

**Output:** `emitted` (bool), `e2eId`. Empty `pix[]` → `{ emitted: false }`
without touching the emitter. A non-empty `pix[]` returns an error. The adapter
does **not** dedup.

The envelope it builds (`providers/efi/adapter/reactor/efi_webhook_received.go`),
addressed to `identities.efi.pix-receive.q`:

```json
{
  "event":       "efi.pix.received",
  "e2eId":       "<endToEndId>",
  "txid":        "<txid>",
  "valor":       "<valor>",
  "status":      "<status>",
  "chave":       "<chave>",
  "horario":     "<horario>",
  "devolucoes":  [],
  "received_at": "<RFC3339 UTC>"
}
```

See [OPERATIONS.md → Webhooks](OPERATIONS.md#webhooks) for who receives Pix
callbacks now.

---

## Resource type: `automatic_webhook`

Canonical prefix `thirdparty.efi.automatic_webhook` · identity
`automatic_webhook.{kind}` · not discoverable. Added in 2.5.0.

EFI keeps one Pix Automatico webhook registration per API application for each
of two endpoints. Neither is keyed by a Pix key, so the identity is the `kind`:

| `kind` | EFI endpoint | Notifications | EFI delivers to | `resource_id` |
|---|---|---|---|---|
| `rec` | `/v2/webhookrec` | recurrence lifecycle | `<webhook_url>/rec` | `webhookrec` |
| `cobr` | `/v2/webhookcobr` | recurring charges | `<webhook_url>/cobr` | `webhookcobr` |

Both capabilities refuse `skip_mtls_validation` and `chave` before any
provider call, never send `x-skip-mtls-checking`, and never call
`/v2/webhook/{chave}` (that is `ensure_webhook_subscription`). The receiver must
authenticate EFI with mTLS. There is **no destroy** for this resource: removing
a registration is not something this adapter offers.

The resource is served by the legacy `adapter.Execute` switch, not by an SDK
Reconciler, because `reconcile.RegisterReconciler` always installs a
`destroy_` operation. `ensure_automatic_webhook` therefore emits its §6.5 event
explicitly (`providers/efi/adapter/automatic_webhook_events.go`).

### `ensure_automatic_webhook`

`GET` the endpoint, then one `PUT {"webhookUrl": ...}` only when nothing is
registered or a different URL is, then `GET` again. Fails unless the readback
equals `webhook_url` exactly (plain string equality, no normalization). Never
retries the PUT. A matching registration
is adopted with no mutation (`changed: false`).

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `kind` | string | yes | `rec` or `cobr`. |
| `webhook_url` | string | yes | Receiver **base** URL: `https://`, a host, no query, fragment, userinfo, escaped path or trailing slash, and the last segment cannot be `rec`, `cobr` or `pix` because EFI appends the delivery suffix. A query is refused because EFI's skip-mTLS mode authenticates with an HMAC in the query. |

**Output:** `ensured`, `changed`, `kind`, `endpoint`, `resource_id`,
`registered`, `webhook_url` (readback, query values redacted), `created_at`
(EFI `criacao`, when returned).

**Event:** `efi.automatic_webhook.ensured` with `resource_id` `webhookrec` or
`webhookcobr` and the request's instance name. Emission is best-effort like the
SDK's: a refused event is logged as a WARN and does not fail the call, because
the readback already proved the provider state. Core accepts the event only
from a principal granted `efi.automatic_webhook.ensured`.

### `observe_automatic_webhooks`

One `GET` of the endpoint. **Read-only.** A 404, or a response without
`webhookUrl`, is reported as `registered: false`, not as an error.

| Input | Type | Required | Notes |
|---|---|:--:|---|
| `kind` | string | yes | `rec` or `cobr`. |
| `expected_webhook_url` | string | no | Turns the read into an exact readback gate: the call fails unless the registration exists and equals this URL. Same URL rules as `webhook_url`. |

**Output:** `kind`, `endpoint`, `resource_id`, `registered`, `webhook_url`,
`created_at`, and `matches_expected: true` when a gate was given and passed.

---

## Idempotency summary

The `execution.idempotent_actions` list (manifest + `spec.go`) marks all
capabilities idempotent **except `create_payout`** (money movement; safety
non-idempotent).

## SDK-only operations

Two canonical operations exist **only** through the SDK reconcile dispatch path
(not in the legacy `SupportedExecuteOperations` list, not user-facing
capabilities): `observe_due_charges` and `destroy_due_charge`. They complete the
`ensure_/observe_/destroy_` triple for the due-charge sub-resource and route
internally to the same `/v2/cob/{txid}` GET + PATCH handlers (BCB Pix exposes
`cob` and `cobv` records under the same paths). You don't dispatch these
directly.

## Legacy aliases

Pre-v2.0.0 names still route through the dispatch path (with a deprecation WARN
log) until v3.0.0 (`LegacyOperationAliases` in `spec.go`):

| Legacy name (v1.x) | Canonical (v2.0.0+) |
|---|---|
| `create_charge` | `ensure_charge` |
| `create_due_charge` | `ensure_due_charge` |
| `get_charge_status` | `observe_charges` (filter `txid`) |
| `get_statement` | `observe_charges` (filter range) |
| `register_webhook_endpoint` | `ensure_webhook_subscription` |
| `unregister_webhook_endpoint` | `destroy_webhook_subscription` |

> New workflows should use the canonical names. The legacy shim is removed in
> integration-efi v3.0.0.
</content>
