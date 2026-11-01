# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 774b50b5802c5646b8a809174aac8215e36d12d7
verified_diff_sha256: bc2e0012bc924c5d3ccab3fcf3a8a358b43c27859aa50b681f01a266bd4cae21
reconciler_schema: 1
verified_at: 2026-10-31
by: Codex
note: Reconciled EFI 2.5.7 optional two-way sealed webhook inputs/results, invocation HMAC equality proofs, RAM-only runtime recipient and fixed public reader. Confirmed ensure requires the actual skip-mTLS-false setter plus exact GET; accepted-PUT postprocessing failure remains held with a safe event attempt. Legacy raw mode and full plaintext B7 semantics remain documented. This stamp binds the exact functional source commit and its diff from qualified 0f03; all tests remain CI-only, with no test-pass or deployment claim.
