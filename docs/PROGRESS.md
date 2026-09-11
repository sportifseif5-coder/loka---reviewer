# Progress Tracker - Loka

> Living execution tracker for the phases, tasks, and steps in
> `docs/ROADMAP.md`. `docs/CONSTITUTION.md` remains the binding contract;
> this file is the checklist of record for *what is done and what is next*.
> Update it every time a step, task, or phase finishes (see protocol below).
>
> **This is the only file that carries status markers.** `docs/ROADMAP.md`
> defines scope and exit criteria; `docs/SESSION_NOTES.md` is the narrative
> log. `make status-source` enforces the split so status cannot drift into
> prose copies elsewhere.

**Last updated:** 2026-09-11 (Session 9)

## Legend and update protocol

- `[x]` done and verified on this repository's own codebase.
- `[~]` partial or implemented differently than planned; the note says why.
- `[ ]` not started.

Protocol:

1. When a step finishes, tick it `[ ]` -> `[x]` in the same change that
   finishes the work. Use `[~]` with a dated note when scope narrowed or a
   regression is found (never silently un-tick completed work).
2. Update the **Last updated** line above with the date and session.
3. Record the narrative in `docs/SESSION_NOTES.md` as usual.
4. A phase is complete only when every task is `[x]`/justified `[~]` **and**
   every exit criterion below it is `[x]`. Do not start a new phase while the
   current phase has open exit criteria (`AGENTS.md` section 2.3).
5. Do not commit `.codegraph/` or other tool output.

## Phase 0 - Foundations - COMPLETE

Exit criteria (defined in `docs/ROADMAP.md`): `[x]` empty-review flow works end to end; `[x]` CI green with
network disabled.

- [x] Go module and workspace layout (`cmd/`, `internal/`).
- [x] CI: lint, unit tests, golden-fixture runner (no network, no keys).
- [x] Storage layer: SQLite migrations; reviews/findings/index/learnings
      schema.
- [x] Config loader with inheritance and validation.
- [x] Desktop shell bootstrap: Wails v2 app window renders a stub workbench.

## Phase 1 - Offline MVP - IN PROGRESS

Exit criteria (defined in `docs/ROADMAP.md`): `[ ]` offline review of this repository with a local model
produces verified findings in < 60s; `[ ]` false-positive rate within noise
budget; `[ ]` applied fixes only on explicit user action.

### 1.1 VCS adapter

- [x] `IsRepo` and working-tree diff vs HEAD (added lines, file statuses,
      untracked files as synthetic all-additions diffs).
- [x] Per-line `Blame` (commit + author).
- [ ] Base-snapshot file content read (today the diff-vs-HEAD is the
      snapshot; no separate content accessor).

### 1.2 Indexer (lightweight graph index)

- [x] Pure-Go stdlib parsing of files, symbols, imports, and intra-package
      reference edges (no new deps, offline, deterministic).
- [x] Repo-scoped store tables with endpoint validation and cascades.
- [x] Incremental `Sync` (hash diff, whole-package reparse on change).
- [x] Query surface: `Callers`, `Callees`, `TypeUses`, `Imports`,
      `FilesTouching`, `ImpactSet` (BFS, depth 2).

### 1.3 Deterministic analyzer pipeline

- [x] Secret detection with bundled patterns (evidence redacted).
- [x] Go lint bridge (`go vet`) scoped to changed Go directories.
- [ ] tree-sitter syntax checks - `[~]` replaced at v1 by pure-Go `go/parser`
      parsing in the indexer; revisit if a non-Go syntax tier is needed.

### 1.4 Rules engine

- [x] `review_rules.md` parsing, path-scoped instructions, built-in defaults.
- [x] Config-driven rule file includes with repo/user resolution and
      warnings-not-errors on missing files.

### 1.5 Findings model

- [x] Finding type with location, reasoning, and evidence (I8).
- [x] Deterministic dedupe and ranking (demoted findings capped and last).
- [ ] Structured review summary (walkthrough + risk) on `ReviewResult`.

### 1.6 LLM provider layer

- [x] Provider interface and recorded-fixture harness (I4; offline CI).
- [x] Ollama local provider.
- [x] llama.cpp (OpenAI-compatible) local provider.
- [x] OpenAI-compatible remote provider (BYO key from local config).
- [x] Router/`Resolve` honoring offline vs online consent (I6) with additive
      degradation to the deterministic baseline.

### 1.7 Agents and context assembly

- [x] Review agent (structured JSON output, parsed and location-verified).
- [x] Verification agent (deterministic added-line verification; demotion).
- [x] Token-budgeted context pack: baseline -> diff -> related.
- [x] Graph-index impact slice (`ImpactSet`) feeding the context pack.

### 1.8 UI workbench (Wails shell)

- [x] Backend surface: `ListRepos`, `ListReviews`, `GetReview`,
      `ReviewContext`, `RepoMode`, store opened once in `startup`.
- [ ] Repo picker and review trigger in `desktop/frontend`.
- [ ] Findings list with severity/source filtering.
- [ ] Inline diff/code view using `ReviewContext`.
- [ ] Apply-suggestion (user-confirmed only, I7).
- [ ] Review history view using `ListRepos`/`ListReviews`.

### 1.9 Phase 1 exit criteria (dogfood on this repo)

- [ ] Offline review with a local model in < 60s.
- [ ] False-positive rate within the noise budget.
- [ ] Applied fixes only on explicit user action (verified through the UI).

## Phase 2 - Online and Agent Mode - NOT STARTED

Exit criteria (defined in `docs/ROADMAP.md`): `[ ]` online review uses user keys with auditability;
`[ ]` agent mode cites external sources; `[ ]` learnings measurably reduce
repeated dismissed categories.

- [ ] Remote OpenAI-compatible providers with BYO keys via the OS keyring.
- [ ] Failover/degradation UX (remote down -> local -> baseline banner).
- [ ] Agent mode: web search/fetch agents with per-repo consent and egress
      log.
- [ ] Learnings: feedback event capture and distillation injected into
      reviews.
- [ ] Fix Agent: proposed diffs for confirmed findings (apply stays explicit).
- [ ] Lint bridges for Python and TypeScript.

## Phase 3 - Hardening and Extension - NOT STARTED

Exit criteria (defined in `docs/ROADMAP.md`): `[ ]` full v1 scope passes all invariants and the quality bar;
`[ ]` emitters are proven emit-only.

- [ ] Benchmarks vs the noise budget on a fixed corpus; precision release
      gate.
- [ ] Advisory DB update flow (user-initiated, no code egress).
- [ ] Git-platform emitter (emit-only PR comments) for GitHub/GitLab; no
      merge capability.
- [ ] Sandbox tightening: capability-scoped agent tool execution.
- [ ] Packaging: signed installers for macOS, Windows, Linux; data
      portability export/import.

## Post-v1 (explicitly not committed)

- [ ] Embeddings / semantic RAG over the codebase.
- [ ] Team features, dashboards, SSO (server component).
- [ ] Cloud SaaS control plane.

## Next action

Phase 1.8 - interactive frontend over the backend surface already in place
(repo picker, review trigger, findings list, inline diff, history), then
Phase 1.9 exit criteria. Narrative and rationale: `docs/SESSION_NOTES.md`.
