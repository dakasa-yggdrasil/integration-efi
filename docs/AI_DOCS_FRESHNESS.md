# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 360bfe6ee150b992e3e07bf36b7cbb2c247584bc
verified_diff_sha256: f57fdbf18750c0c8bbea7129f5808caf5fe8ab34b3ce78792d669876f987646e
reconciler_schema: 1
verified_at: 2026-10-31
by: Codex
note: Reconciled EFI 2.5.7 optional two-way sealed webhook inputs/results, invocation HMAC equality proofs, RAM-only runtime recipient and fixed public reader. Confirmed ensure requires a nonempty complete registration page, the actual skip-mTLS-false setter and exact GET; accepted-PUT postprocessing failure remains held with a safe event attempt. The confidential empty-page refusal preserves legacy raw empty-page behavior and full plaintext B7 semantics. This stamp binds the exact functional source commit and its diff from qualified 0f03; all tests remain CI-only, with no test-pass or deployment claim.

sdk010_original_context_drain_review: Reconciled original caller context, exact Run/WaitDrained ownership, actual listener-error cancellation, retained ABI/provider/idempotency/auth behavior, explicit example grace and separate runtime desired adoption. CI-only actual provider/Reply/settlement/startup checks qualify Source; no local tests/builds/imports or Native occurred. Genuine551 input bytes remain equal to the signed code commit and raw generated provenance is retained.
