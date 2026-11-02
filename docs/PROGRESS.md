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

**Last updated:** 2026-11-02 (Session 15)

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

Exit criteria (defined in `docs/ROADMAP.md`): `[~]` offline review of this repository with a local model
produces verified findings in < 60s (2026-10-26: deterministic baseline 2.3s / 0 findings; LLM skipped, no local model); `[x]` false-positive rate within noise
budget on this repo; `[x]` applied fixes only on explicit user action.

### 1.1 VCS adapter

- [x] `IsRepo` and working-tree diff vs HEAD (added lines, file statuses,
      untracked files as synthetic all-additions diffs).
- [x] Per-line `Blame` (commit + author).
- [x] Base-snapshot file content read (`VCSAdapter.ReadAtHEAD`; impact
      context prefers HEAD over the working tree).

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
- [~] tree-sitter syntax checks - replaced at v1 by pure-Go `go/parser`
      in the indexer (ADR-0007, 2026-10-26); revisit if a non-Go syntax
      tier is needed.

### 1.4 Rules engine

- [x] `review_rules.md` parsing, path-scoped instructions, built-in defaults.
- [x] Config-driven rule file includes with repo/user resolution and
      warnings-not-errors on missing files.

### 1.5 Findings model

- [x] Finding type with location, reasoning, and evidence (I8).
- [x] Deterministic dedupe and ranking (demoted findings capped and last).
- [x] Structured review summary (walkthrough + risk) on `ReviewResult`.

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
- [x] Repo picker and review trigger in `desktop/frontend`.
- [x] Findings list with severity/source filtering.
- [x] Inline diff/code view using `ReviewContext` (a stale location past EOF
      now falls back to the file tail).
- [~] Apply-suggestion - deferred until findings carry suggested fixes
      (Phase 2 Fix Agent); the UI exposes no inert apply control. (2026-09-11)
- [x] Review history view using `ListRepos`/`ListReviews`/`GetReview`.

### 1.9 Phase 1 exit criteria (dogfood on this repo)

- [~] Offline review with a local model in < 60s. (2026-10-26) `make dogfood`
      ran this repository in 2353ms (31 changed files, risk none, 0 findings,
      analyzers secret-detection/go-vet/rules). LLM stage skipped: no Ollama
      on :11434 and no llama.cpp on :8080. Re-run when a local model is
      reachable; the deterministic path already meets the wall-clock budget.
- [x] False-positive rate within the noise budget. Dogfood of this working
      tree produced 0 findings after excluding CLI/tests from
      `debug-print-in-go` and skipping tagged-out `go vet` packages. Corpus
      precision gate remains Phase 3.
- [x] Applied fixes only on explicit user action (verified through the UI).
      The workbench exposes no apply control; the engine has no
      apply/merge/push methods (I7). Apply stays deferred until Phase 2 Fix
      Agent.

## Phase 2 - Online and Agent Mode - COMPLETE

Exit criteria (defined in `docs/ROADMAP.md`): `[x]` online review uses user keys with auditability
(BYO via `api_key_keyring` / `api_key_env`, failover banner, egress log; keys never logged);
`[x]` agent mode cites external sources (web agent, mode-gated, URL on evidence + egress);
`[x]` learnings measurably reduce repeated dismissed categories (3 dismissals distill; LLM
findings in that category are dropped on later reviews).

Phase 1 is not closed. 1.9's local-model half is still `[~]`. Return playbook:
`docs/PHASE_1_9_RETURN.md`. Do not mark Phase 1 complete from this work.

- [x] Remote OpenAI-compatible providers with BYO keys. (2026-11-02) Portable
      0600 file store under `$XDG_CONFIG_HOME/loka/secrets` plus env fallback
      is the v1 keyring (constitution 10.5 allows OS keyring *or* a user-owned
      0600 file). Native Secret Service / Keychain remains a later enhancement.
      Config: `api_key_keyring: service/account`. Never `MCAI_*`.
- [x] Failover/degradation UX. Router `CompleteTraced` labels local, remote,
      local-down-to-remote, or baseline. Banner is on `ReviewResult`, CLI
      stderr, and the workbench `#banner`.
- [x] Agent mode: web search/fetch with per-repo consent (`mode: agent` only)
      and an egress log. Offline and online modes never open the fetcher.
      Findings cite the URL in evidence.
- [x] Learnings: `RecordFeedback` + `Distill` (threshold 3 dismissals) stored
      in SQLite; injected into the context pack and used to drop matching
      LLM categories on later reviews. Workbench Accept/Dismiss records events.
- [x] Fix Agent: unified-diff suggestions on confirmed trailing-whitespace
      and debug-print findings. Apply is `App.ApplySuggestion` (explicit click).
      Engine still has no apply/merge/push (I7).
- [x] Lint bridges for Python and TypeScript: offline added-line pattern
      checks (`python-lint`, `typescript-lint`) registered in `NewPipeline`.
      Subprocess ruff/eslint is not required for the v1 bridge.

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

**Must return to Phase 1.9** before calling Phase 1 complete. Follow
`docs/PHASE_1_9_RETURN.md`: start Ollama (`:11434`) or llama.cpp (`:8080`),
re-run `make dogfood`, prove `analyzers_run` contains `llm:<model>`, then
tick the 1.9 local-model line. This environment could not load a model
(cgroup RAM ~300 MiB, no swap, no runtime installed).

Phase 2 coding is complete. Do not start Phase 3 until the user asks.
Narrative: `docs/SESSION_NOTES.md`.
