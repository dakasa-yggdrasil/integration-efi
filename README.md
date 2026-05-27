# integration-efi

Public Yggdrasil adapter for Brazilian Pix (Banco Central PIX API + EFI overlay).

## Status

v1.0.0 — production-ready. Replaces the EFI provider in `dakasa-co/integration-webhooks-external`
and the in-process EFI client in `dakasa-app-fe/backend/dakasa-identities`.

## Capabilities

11 capabilities (10 user-dispatched + 1 webhook reactor). See `manifest/capabilities/`.

## Run

See `docker-compose.standalone.yml` for local dev; production deploy via Yggdrasil binding (`manifest/integration_instance.example.json`).
