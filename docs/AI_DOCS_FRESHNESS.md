# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 41955faeb832c9e215fafa2a74026985a83bdc73
verified_diff_sha256: d00d0a03052a1814bdc2d7d47bcd4c311ac987ca463be44745b7eec4b6ab82ac
reconciler_schema: 1
verified_at: 2026-10-30
by: Codex
note: Reconciled EFI 2.5.6 raw single/LIST observations, compatibility aliases and pre-sanitization URL/page provenance. Generated context describes the exact functional source commit; all tests remain CI-only.
