# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 58db9473e3a15c508edd5733d3623577966c9967
verified_diff_sha256: c6397254c75058477877f3d2cec3b9344f7aef770e04663c94285ae643601a98
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.5 and the API-versus-callback certificate boundary. Generated context covers the exact functional source commit; provider callback, commercial and account evidence remain separate gates.
