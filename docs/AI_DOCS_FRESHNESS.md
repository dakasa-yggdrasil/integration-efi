# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 3ae85fdb07dbcbcb8200b7c3d9207586ed57602c
verified_diff_sha256: cae4c1ee9e6514c004ff34cbd7cf59472d35fe3736631badfc85348a01b865d9
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.4 and the TLS 1.3 clientless refusal proof. Generated context covers the exact functional source commit; commercial and account gates remain unchanged.
