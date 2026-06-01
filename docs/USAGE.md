# Usage — integration-efi

End-to-end journey: install the adapter → configure an instance → run your first
workflow → verify the run. Every command and capability name here is verified
against the manifests and adapter code in this repo.

← Back to the [README](../README.md) · part of
[Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core). See also
[CONFIGURATION.md](CONFIGURATION.md) · [CAPABILITIES.md](CAPABILITIES.md).

---

## 1. Install the integration

The repo publishes a `yggdrasil-quickstart.yaml` bundle that registers the `efi`
family + type and seeds an example instance. Install it via the Yggdrasil CLI:

```bash
yggdrasil install dakasa-yggdrasil/integration-efi
```

This applies:

- `family/manifest.json` → family `efi` (domain `payments`, Apache-2.0)
- `manifest/integration_type.json` → type `efi` (adapter, schemas, resource types)
- `manifest/integration_instance.example.json` → example instance `efi-prod`
- a deploy ref for `ghcr.io/dakasa-yggdrasil/integration-efi`

> The quickstart's `image_tag` is stale (`v1.0.0`). Pin a real published tag
> (`sha-<short>` from the `release` workflow, or a `vX.Y.Z` tag) instead — the
> running binary advertises adapter version `2.4.0`. See
> [CONFIGURATION.md → Version truth](CONFIGURATION.md#version-truth).

## 2. Configure the instance

Supply real credentials and (optionally) override instance config. Minimal
instance manifest:

```json
{
  "namespace": "global",
  "name": "efi-prod",
  "spec": {
    "type_ref": { "namespace": "global", "name": "efi" },
    "credentials": {
      "efi_client_key_id":      "<from your secret store>",
      "efi_client_secret":      "<from your secret store>",
      "efi_certificate_base64": "<base64 P12, from your secret store>"
    },
    "config": {
      "base_url":     "https://pix.api.efipay.com.br",
      "sandbox":      false,
      "mtls_enabled": true,
      "webhook_port": 9079
    },
    "discovery": { "enabled": false }
  }
}
```

`efi_client_secret` and `efi_certificate_base64` are secrets. Use
`https://pix-h.api.efipay.com.br` + `sandbox: true` for EFI homologation. Full
field reference: [CONFIGURATION.md](CONFIGURATION.md).

## 3. Run your first workflow — create a Pix charge

`ensure_charge` creates an immediate Pix charge (`cob`). A complete workflow:

```yaml
apiVersion: yggdrasil.io/v1
kind: Workflow
metadata:
  name: efi-charge-example
  namespace: global
spec:
  steps:
    - name: create-charge
      capability: ensure_charge
      integration_instance_ref:
        namespace: global
        name: efi-prod
      input:
        valor:
          original: "10.00"
        chave: "pix-key@dakasa.me"
        expiracao: 3600
```

Apply it:

```bash
yggdrasil apply -f efi-charge-example.yaml
```

Or trigger a one-off execute run directly against core's HTTP API (the shape the
staging runbook uses):

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $YGGDRASIL_WORKFLOW_RUN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "workflow": {"name": "integration-execute", "namespace": "global"},
    "inputs": {
      "integration_instance_ref": {"namespace": "global", "name": "efi-prod"},
      "capability": "ensure_charge",
      "input": {
        "valor": {"original": "10.00"},
        "chave": "pix-key@dakasa.me"
      }
    }
  }' \
  "$YGGDRASIL_CORE_BASE_URL/api/v1/workflow-runs" | jq
```

**Expected output** from `ensure_charge`:

```json
{
  "txid":          "...",
  "location":      "https://...",
  "pixCopiaECola": "00020126...",
  "status":        "ATIVA",
  "created_at":    "2026-06-01T..."
}
```

## 4. Verify the run

1. Check the workflow run completed `ok` in core (UI or API).
2. The adapter exposes Prometheus metrics — a successful charge increments
   `efi_request_duration_seconds{op="cob"}` and leaves
   `efi_request_errors_total{op="cob"}` flat:

   ```bash
   curl -s "$PROM/api/v1/query?query=efi_request_errors_total" | jq '.data.result'
   ```

3. `efi_adapter_up == 1` confirms the worker is healthy. See
   [OPERATIONS.md](OPERATIONS.md).

## 5. Common follow-on operations

| Goal | Capability | Key inputs |
|---|---|---|
| Check a charge / pull a statement | `observe_charges` | `{txid}` **or** `{inicio, fim}` |
| Cancel a charge | `destroy_charge` | `{txid}` |
| Due-date (boleto-style) charge | `ensure_due_charge` | `txid, valor, chave, calendario, devedor` |
| Refund a Pix | `refund_charge` | `e2eId, id, valor` |
| Send a payout | `create_payout` | `idEnvio, valor, pagador, favorecido` |
| Register a webhook | `ensure_webhook_subscription` | `chave, webhook_url` |
| List/inspect webhooks | `observe_webhook_subscriptions` | `{chave}` or empty |

Full input/output schemas: [CAPABILITIES.md](CAPABILITIES.md).

## 6. Receiving inbound Pix callbacks

After `ensure_webhook_subscription`, EFI POSTs Pix callbacks to the adapter's
mTLS webhook listener on `:9079` (`/efi/webhook/pix`). The `efi_webhook_received`
reactor normalizes each event and emits it onto the bus
(`identities.efi.pix-receive.q`). This is **not** something you call — see
[OPERATIONS.md → Webhooks](OPERATIONS.md#webhooks) and
[CAPABILITIES.md → efi_webhook_received](CAPABILITIES.md#efi_webhook_received--reactor).

## 7. Local dev loop

```bash
task up      # docker compose: rabbitmq + adapter (builds the image)
task logs    # follow logs
task down    # tear down
```

For mock/test instances set `EFI_MTLS_ENABLED=false` to skip the cert
requirement. See [DEVELOPMENT.md](DEVELOPMENT.md).
</content>
