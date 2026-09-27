# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 466cd4a77b70e4651766139222c9af1601aa8af5
verified_diff_sha256: 7e93fe0ae677456b32e743b836bd0d6d531c1e51fcf9ca73f0d75ecf0c10c134
reconciler_schema: 1
verified_at: 2026-09-27
by: Codex
note: Reconciled integration-efi 2.5.2 and its read-only Pix Automatico receiver readiness proof. Generated context now covers the exact functional source commit. Commercial eligibility and account identity remain outside the technical proof.
