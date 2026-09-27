# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: b7de5329343c0083fa85397bc79c613bc793261b
verified_diff_sha256: 4504a98572ae6430516e055acd0dfde006851b82caab82a3392e68a5ddf4977d
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.4 and the TLS 1.3 clientless refusal proof. Generated context covers the exact functional source commit; commercial and account gates remain unchanged.
