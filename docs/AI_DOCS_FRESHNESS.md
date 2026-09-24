# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 5d9f5251a2d7f2e38654ad60b48240bf33fb4ce9
verified_at: 2026-09-24
by: Claude
note: Reconciled the Pix Automatico automatic_webhook capabilities, the 2.5.0 release and the merged 2.4.1 line.
