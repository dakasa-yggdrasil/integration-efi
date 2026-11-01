# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 6675d2c51959f7d44aa68134e6e591c7aacf52ea
verified_diff_sha256: 14e19a4e6c508ab54ce8f1c052ecd49006bc0fde43704684c3b16cd431e1fce5
reconciler_schema: 1
verified_at: 2026-10-31
by: Codex
note: Reconciled EFI 2.5.7 optional two-way sealed webhook inputs/results, invocation HMAC equality proofs, RAM-only runtime recipient and fixed public reader. Confirmed ensure requires a nonempty complete registration page, the actual skip-mTLS-false setter and exact GET; accepted-PUT postprocessing failure remains held with a safe event attempt. The confidential empty-page refusal preserves legacy raw empty-page behavior and full plaintext B7 semantics. This stamp binds the exact functional source commit and its diff from qualified 0f03; all tests remain CI-only, with no test-pass or deployment claim.
