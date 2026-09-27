# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 5d415321c5aa4ba6b2dd41451b952c5791f62075
verified_diff_sha256: 98272690eca4776628e652b8029c88126637b6f470973f944c2ee24be295e811
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.5 after preserving its response contract. Generated context covers the exact functional source commit; receiver server-certificate evidence remains separate from provider callback, commercial and account gates.
