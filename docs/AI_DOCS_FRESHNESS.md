# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 4a9b59e2466468a4f99b31839c2e8bfcbd6c1a03
verified_diff_sha256: 507bf76e00706dd3c25881c44466824369255fb46ed3e814f069e41c166f735a
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.3 and the SDK bridge input isolation fix. Generated context now covers the exact functional source commit; the automatic webhook readiness contract and commercial gate remain unchanged.
