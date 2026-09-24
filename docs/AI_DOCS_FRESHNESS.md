# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 534ae7b7e946e9810720386fa3319e7396e4403b
verified_at: 2026-09-24
by: Claude
note: Reconciled the removal of the dead workflow-run dispatch and the 9079 webhook listener on top of 2.5.0, released as 2.5.1.
