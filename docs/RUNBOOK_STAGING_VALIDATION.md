# integration-efi — Staging Validation Runbook

## Pre-flight (T-30min)

1. Verify pod `integration-efi-validation` is Running (1/1):
   ```
   kubectl -n validation get pods -l app=integration-efi
   ```
2. Verify `efi_adapter_up == 1` in Prometheus:
   ```
   curl -sS "$PROM/api/v1/query?query=efi_adapter_up%7Bnamespace%3D%22validation%22%7D" | jq '.data.result'
   ```
3. Verify the validation EFI cert is loaded (logs):
   ```
   kubectl -n validation logs deploy/integration-efi | grep "integration-efi adapter starting"
   ```

## Test 1: 100 create_charge calls

Send 100 charges to EFI homologation via the validation adapter. `CORE_URL` is
the yggdrasil-core base URL and `CORE_TOKEN` a core API bearer allowed to start
workflow runs; neither is an adapter setting.

```bash
for i in $(seq 1 100); do
  curl -sS -X POST \
    -H "Authorization: Bearer $CORE_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
      "workflow": {"name": "integration-execute", "namespace": "global"},
      "inputs": {
        "integration_instance_ref": {"namespace": "validation", "name": "efi-validation"},
        "capability": "create_charge",
        "input": {
          "valor": {"original": "1.00"},
          "chave": "dakasa-staging@dakasa.me"
        }
      }
    }' \
    "$CORE_URL/api/v1/workflow-runs" | jq '.id'
done
```

**Assertions**:
- All 100 return HTTP 200 with `txid` non-empty + `status: ATIVA`.
- `efi_request_errors_total{op="cob"}` does not increment.
- `efi_request_duration_seconds{op="cob"}` p99 < 500ms.

## Tests 2 and 3: webhook callbacks and duplicate delivery (retired)

These tests posted simulated EFI callbacks to exercise the adapter's webhook
listener, its `efi_webhook_received_total` metric and the `publish_message` path
into `identities.efi.pix-receive.q`. 2.5.1 removed that listener and dispatch:
the workflow was never registered and the port was never routed. Callbacks go
to the service behind the registered `webhook_url`, so validate delivery and
dedup with that service's own runbook, not this one.

## Acceptance gate

If Test 1 passes and zero alerts fire in 30min observation: cutover to prod is approved.

## Rollback

Reverse the validation deploy: delete the validation instance + type manifests via Yggdrasil. Pod is auto-removed when its binding is deleted.
