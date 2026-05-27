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

Send 100 charges to EFI homologation via the validation adapter:

```bash
for i in $(seq 1 100); do
  curl -sS -X POST \
    -H "Authorization: Bearer $YGGDRASIL_WORKFLOW_RUN_TOKEN" \
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
    "$YGGDRASIL_CORE_BASE_URL/api/v1/workflow-runs" | jq '.id'
done
```

**Assertions**:
- All 100 return HTTP 200 with `txid` non-empty + `status: ATIVA`.
- `efi_request_errors_total{op="cob"}` does not increment.
- `efi_request_duration_seconds{op="cob"}` p99 < 500ms.

## Test 2: 100 webhook callbacks

Use `curl` to simulate EFI callbacks from a homologation-attached endpoint:

```bash
for i in $(seq 1 100); do
  E2EID="E2E-staging-$(uuidgen)"
  curl -sS -X POST \
    --cert /etc/efi/staging-client.pem \
    --key /etc/efi/staging-client.key \
    -H "Content-Type: application/json" \
    -d "{\"pix\":[{\"endToEndId\":\"$E2EID\",\"chave\":\"dakasa-staging@dakasa.me\",\"valor\":\"1.00\",\"status\":\"REALIZADO\"}]}" \
    "https://webhook-h.dakasa.me/efi/webhook/pix"
done
```

**Assertions**:
- All 100 return 202 Accepted on first delivery.
- `efi_webhook_received_total{status="received"}` = 100.
- `identities.efi.pix-receive.q` message count = 100 (or 0 if consumer drained).
- `identities.efi.pix-receive.q.dlq` message count = 0.

## Test 3: Duplicate delivery

Send the same payload twice — verify dedup at identities consumer:

```bash
PAYLOAD='{"pix":[{"endToEndId":"E2E-dup-test","chave":"dakasa-staging@dakasa.me","valor":"1.00","status":"REALIZADO"}]}'
curl -sS -X POST --cert ... -d "$PAYLOAD" https://webhook-h.dakasa.me/efi/webhook/pix # expect 202
curl -sS -X POST --cert ... -d "$PAYLOAD" https://webhook-h.dakasa.me/efi/webhook/pix # expect 202 (adapter doesn't dedup)
```

Query identities DB:
```sql
SELECT count(*) FROM webhook_event_efi WHERE e2e_id = 'E2E-dup-test';
```
Expected: 1 (UNIQUE constraint kept duplicate out).

## Acceptance gate

If all 3 tests pass + zero alerts fired in 30min observation: cutover to prod is approved.

## Rollback

Reverse the validation deploy: delete the validation instance + type manifests via Yggdrasil. Pod is auto-removed when its binding is deleted.
