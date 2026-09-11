# Roadmap - Loka

> Milestones are gated by exit criteria, not dates. A milestone is done when
> its exit criteria pass on this repository's own codebase (constitution
> section 10.4).
>
> Live per-task status lives in `docs/PROGRESS.md`; this file defines scope
> and exit criteria.

## Phase 0 - Foundations

Scope: repository scaffolding, module layout, CI, storage schema, config.

- Go module + workspace layout (`cmd/`, `internal/`).
- CI: lint, unit tests, golden-fixture runner (no network, no keys).
- Storage layer: SQLite migrations, schema for index/findings/learnings.
- Config loader with inheritance and validation.
- Desktop shell bootstrap: Wails app window renders a stub workbench.

Exit criteria: empty-review flow works end to end (open repo, run review,
zero findings, results persisted); CI green with network disabled.

## Phase 1 - Offline MVP

Scope: deterministic review, rules, local LLM, basic UI.

- VCS adapter: diff, blame, base snapshot (read-only).
- Indexer: files, symbols, references; incremental updates; `ImpactSet`.
- Analyzer pipeline: tree-sitter syntax checks, one lint bridge (Go),
  secret detection with bundled patterns.
- Rules engine: `review_rules.md`, path-scoped instructions, defaults.
- Findings model + ranking + dedupe; structured review summary.
- LLM provider layer: Ollama + llama.cpp local providers, fixture adapter.
- Review Agent + Verification Agent; context pack assembly.
- UI: repo picker, run review, findings list, inline diff, apply-suggestion
  (user-confirmed), review history.

Exit criteria: on this repo, offline review with local model produces
verified findings in < 60s; false-positive rate within noise budget; applied
fixes only on explicit user action.

## Phase 2 - Online and Agent Mode

Scope: remote providers, web-capable agents, richer analysis.

- Remote OpenAI-compatible providers with BYO keys (OS keyring).
- Failover/degradation UX (remote down -> local -> baseline banner).
- Agent mode: web search/fetch agents with per-repo consent and egress log.
- Learnings: feedback event capture + distillation; injected into reviews.
- Fix Agent: proposed diffs for confirmed findings (apply stays explicit).
- Lint bridges for a second and third language (Python, TypeScript).

Exit criteria: online review uses user keys with auditability; agent mode
cites external sources; learnings measurably reduce repeated dismissed
categories.

## Phase 3 - Hardening and Extension

Scope: performance, security, portability, optional emitters.

- Benchmarks vs. noise budget on a fixed corpus; precision release gate.
- Advisory DB update flow (user-initiated, no code egress).
- Git-platform emitter (emit-only PR comments) for GitHub/GitLab; no merge
  capability.
- Sandbox tightening: capability-scoped agent tool execution.
- Packaging: signed installers for macOS, Windows, Linux; data portability
  export/import.

Exit criteria: full v1 scope passes all invariants and quality bar
(constitution sections 5, 11); emitters are proven emit-only.

## Post-v1 (explicitly not committed)

- Embeddings / semantic RAG over the codebase.
- Team features, dashboards, SSO (would require a server component).
- Cloud SaaS control plane.
