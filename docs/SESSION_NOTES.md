# Session Notes - Loka

> Companion to `AGENTS.md` section 2. Records what was done and what is next.
> Updated at the end of every session. History is authoritative; this file is
> the summary.

## Session 6 - 2026-09-04: Lightweight graph index (files, symbols, refs)

### State

- Phase: **Phase 1 (Offline MVP)**. Deterministic baseline, provider layer,
  and agent layer are done and CI-green. The indexer's files/symbols/reference
  extraction with incremental updates is now built and CI-green; remaining
  Phase 1 scope: indexer `ImpactSet(diff)`/query surface feeding context
  packs, and the UI workbench.
- Branch `master`, remote `origin`. Pushed via the env-stripped push command.

### What was built

- `internal/store` index tables rebuilt repo-scoped (v6 migration) so one
  shared per-user DB can hold several repositories: `index_files`
  (PK repo_path+path), `index_symbols` and `index_refs` (symbols keyed
  (repo_path, file_path, name); refs as symbol->symbol edges with FK cascades
  to files/symbols), `index_imports`. `PRAGMA foreign_keys=ON` in `Open`.
- `internal/store/index.go`: `ReplaceDirIndex` (transactional full replace of
  one package directory; validates every ref endpoint resolves to a symbol in
  the written set), `DeleteDirIndex`, `IndexHashes` (incremental driver),
  `DirIndex` load with denormalized refs. Fixed the load query alias bug
  (`src.file_path` -> `src.path`).
- `internal/indexer`: pure-Go stdlib backend (`go/parser` + `go/ast`; no new
  deps), fully offline and deterministic.
  - `Sync`: discovers `.go` files (hash via sha256), diffs against stored
    hashes, and re-parses only stale **package directories** (whole-dir
    replace keeps a package's rows internally consistent). Dry-run with a nil
    store computes the same summary without persisting.
  - `golang.go`: parse + file-scope symbols (funcs, methods qualified by
    receiver base type e.g. `Store.Save`, types/structs/interfaces, consts,
    vars; blank `_` skipped) and imports; `packageTable` groups symbols by
    package clause so external `_test` packages resolve against their own
    table.
  - `refs.go`: structural intra-package reference resolver (no go/types).
    Walks each file with lexical shadow scopes (receiver/params, short vars,
    range vars, blocks, clauses) so locals never masquerade as package
    symbols; records `call` edges for direct calls and `use` edges for other
    references (incl. cross-file, embedded types, signature/receiver type
    uses); drops self-edges and duplicates; parse failures are skipped and
    reported, never fatal.
- Tests (`internal/indexer/indexer_test.go`): fixture-module assertions on
  the exact extracted symbol/ref set (including shadow-guard negatives),
  run-to-run determinism, broken-file skip, incremental no-op on unchanged
  resync + single-dir reparse on touch + delete-of-empty-dir, persistence
  round-trip, dry-run summary. All offline-safe.
- Quality gates: `make lint` and `go test ./... -count=1` green.

### Next session

1. Indexer query surface: `Callers(symbol)`, `Callees(symbol)`,
   `TypeUses(type)`, `Imports(file)`, and `ImpactSet(diff)` over the stored
   edges (architecture 5.3), then feed the slice into agent context packs
   (architecture 4.2 relevance ordering).
2. UI workbench: findings list + inline diff with apply-suggestion
   (user-confirmed, I7), review history in the Wails shell.

## Session 5 - 2026-09-02: Agent layer (Review + Verification)

### State

- Phase: **Phase 1 (Offline MVP)**. Deterministic baseline, LLM provider
  layer, and the agent layer are done and CI-green. Remaining Phase 1 scope:
  UI workbench (repo picker, findings list, inline diff, apply-suggestion,
  history) and the lightweight graph index.
- Branch `master`, remote `origin`. Pushed via the env-stripped push command.

### What was built

- `internal/agent` (architecture section 8): `Agent` interface + `Request`
  (RepoPath, Changed, Baseline, Budget) and `Result` (Findings, Model, Usage,
  Invalid, Demoted). `splitBudget` divides review tokens into a 3:1
  context:output split.
- `ReviewAgent`: assembles the token-budgeted context pack, completes through
  the provider router (localOnly in offline mode, I6), parses the JSON
  response into candidates (I8: entries without location+message are dropped
  and counted), then runs the verification stage. No network path of its own.
- `packContext` (architecture 4.2): diff added lines + deterministic baseline.
  Baseline is written first and never truncated; the diff tail is dropped on
  budget exhaustion with a truncation marker. (Full relevance-ordering waits
  for the graph index.)
- `VerificationAgent`: deterministic false-positive check. A candidate whose
  file+line lands on an added line is confirmed (confidence 0.6); anything
  else is **demoted, never deleted** (severity capped at info, confidence 0.2,
  `Finding.Demoted=true`). Store v5 migration adds the `demoted` column.
- Engine `llmStage` now delegates to `agent.NewReviewAgent()`; the old
  `internal/review/llm.go` context/prompt/parse code was relocated into the
  agent package (files git-removed).
- Tests: agent unit tests (verify demote-not-delete, parse drops invalid,
  pack truncates-by-budget but keeps baseline, full pipeline via fixture
  provider, no-provider and unmatched-fixture errors, budget split) and engine
  tests rewritten (merge-verified, demote-non-added-line, offline excludes
  remote, failure keeps baseline). All offline-safe; CI incl. netns is green.
- E2E re-verified on `/tmp/e2e`.

### Next session

1. UI workbench: findings list + inline diff rendering with apply-suggestion
   (user-confirmed, I7), review history in the Wails shell.
2. Lightweight graph index (`internal/indexer`): files/symbols/refs tables
   (schema v2 exists), incremental updates, `ImpactSet(diff)` feeding richer
   context packs.

## Session 4 - 2026-09-01: LLM provider layer

### State

- Phase: **Phase 1 (Offline MVP)**. Deterministic baseline (Session 3) plus
  the LLM provider layer are done and CI-green. Agent layer and UI remain.
- Branch `master`, remote `origin`. Pushed via the env-stripped push command.
- CI (core + desktop, incl. real-netns offline tests) green on GitHub for
  the previous commit. The offline-step battle: `setup-go` places go 1.25 on
  PATH, but `sudo unshare` resets PATH (found go 1.24.13) and HOME (cold
  module cache). Fix: preserve PATH, HOME, and GOMODCACHE through sudo and set
  `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` for the isolated run.

### What was built

- `internal/provider`: `Provider` interface (Name/Local/Available/Complete),
  `Router` with ADR-0003 routing (local always first, then remote unless
  localOnly; failover on unavailable/error; `ErrNoProvider` when all fail).
  OpenAI-compatible `client` with injectable `http.Transport` so request/parse
  logic is tested without a socket (offline CI). Local providers `Ollama`
  (localhost:11434) and `LlamaCpp` (localhost:8080), remote
  `OpenAICompatible` (BYO key via `api_key_env`), `FixtureProvider` (recorded
  transcripts, empty-prompt wildcard, never opens a socket, no keys). Factory
  `FromConfig`/`Resolve`; unknown provider names are errors that degrade at
  CLI level, never abort.
- Engine LLM stage (`internal/review/llm.go` + wiring): `RegisterProviderRouter`;
  runs after the deterministic baseline; offline mode gates to local-only
  (I6); every failure path adds a degradation and ships the baseline (I2/I3).
  `packContext` (changed files' added lines + baseline findings) feeds the
  prompt; output parsed as JSON findings, each validated for location+message
  (I8), labeled `Source=llm`, `Confidence=0.5` (ranked below baseline).
- Tests: router policy (local-preferred, failover, offline-excludes-remote,
  empty), fixture replay + wildcard, HTTP request shape/parse via fake
  transport, ping semantics, FromConfig; engine tests for merge, offline
  remote exclusion, failure-degrade-keeps-baseline; unit tests for
  parseLLMFindings and packContext.
- CLI `loka review`: builds the router from config, registers it; provider
  misconfiguration logs a warning and disables the LLM stage.
- E2E re-verified on `/tmp/e2e`: 5 baseline findings persist; with no Ollama
  running the LLM stage degrades ("no provider available") and the baseline
  still ships.

### Next session

1. Agent layer (`internal/agent`): Review Agent + Verification Agent on top of
   the provider router; richer context pack assembly with token budgeting.
2. UI: findings list + inline diff in the Wails workbench.

### Tooling (2026-09-01)

- **Codegraph installed** (`@colbymchenry/codegraph` v1.6.0 via npm -g to
  /usr/local/bin). MCP server registered in `~/.config/opencode/opencode.json`
  (`mcp.codegraph` -> `codegraph serve --mcp`, enabled). Index built for this
  repo (`codegraph init`, `.codegraph/` gitignored). Telemetry off. Restart
  opencode for the MCP server to load.
- **Archify skill installed** at `~/.config/opencode/skills/archify/`
  (cloned from `tt-a1i/archify`, copied the `archify/` skill dir; SKILL.md
  frontmatter valid, renderer runs with bundled deps). Restart opencode for
  the skill to load.

## Session 3 - 2026-08-31: Phase 1 deterministic core

### State

- Phase: **Phase 1 (Offline MVP), deterministic baseline complete**. VCS
  adapter, rules engine, deterministic analyzers, engine wiring, and the
  store evidence column are done and verified end to end. LLM provider layer
  and agent layer are the remaining Phase 1 scope (deferred to next session).
- Branch: `master`. Remote: `origin`
  (`https://github.com/sportifseif5-coder/loka---reviewer`).
- Push requires stripping the injected credential helper:
  `env -u GIT_CONFIG_COUNT -u GIT_CONFIG_KEY_0 -u GIT_CONFIG_VALUE_0 -u GIT_CONFIG_KEY_1 -u GIT_CONFIG_VALUE_1 git push`.

### What was built

- `internal/vcs`: read-only git adapter (`git` CLI shell-out). `WorkingDiff`
  (staged+unstaged via `git diff HEAD`, untracked files via synthetic
  all-additions diff), `Blame`, `IsRepo`, added-line extraction
  (`ParseChangedFiles`). Tests build a throwaway git repo.
- `internal/rules`: `review_rules.md` front-matter parser (name, paths,
  severity, pattern), `**` globs that also match root-level files, model
  assisted vs deterministic, default low-noise rules (merge-conflict-markers,
  trailing-whitespace, debug-print-in-go/js). Golden-free but unit-tested.
- `internal/analyzer`: moved `Analyzer`/`AnalysisUnit` out of `review` to
  avoid an import cycle. `SecretDetector` (bundled regexes: AWS, GitHub,
  private keys, Slack, Stripe, generic keys; evidence redacted; lock files
  ignored) and `GoVetBridge` (`go vet` per changed package, diagnostics
  parsed from stderr, degrades when not a module).
- `internal/review` engine: wired VCS -> analyzers -> rules -> dedupe -> rank
  -> persist. Findings get IDs; dedupe keyed on file:line:rule.
- `internal/store`: v4 migration adds `findings.evidence` (JSON array).
- CLI `loka review`: registers git VCS, both analyzers, default rules +
  `review_rules.md` include resolution. Desktop `App.Review` now runs the
  real engine.
- CI offline step hardened: `GOTOOLCHAIN=local`, `GOPROXY=off`, and the sudo
  netns branch now preserves PATH (runner had go 1.24.13 vs required 1.25).

### Verified

- E2E on a fixture repo (`/tmp/e2e`): AWS secret (critical, redacted), go vet
  printf diagnostic, generic API key, debug-print rules all surfaced and
  persisted; 3 analyzers ran, no degradations.
- `make ci` green locally including the real-netns offline run.
- Root-cause fixes along the way: staged-only changes were invisible to
  `git diff` (now `git diff HEAD`); empty finding IDs broke persistence;
  `**/*.go` glob missed root files.

### Next session

1. LLM provider layer (`internal/provider`): Provider interface, Fixture
   adapter (replays recorded transcripts, no network/keys), Ollama + llama.cpp
   local adapters.
2. Agent layer (`internal/agent`): Review Agent + Verification Agent, context
   pack assembly (diff hunks + findings + rules, token-budgeted).
3. UI: findings list + inline diff rendering in the Wails workbench.
4. Confirm CI green on GitHub for the Phase 1 deterministic commit.

## Session 2 - 2026-08-28: Phase 0 foundations

### State

- Phase: **Phase 0 (Foundations), feature-complete**. All scope items landed:
  module layout, CI, storage schema, config loader, Wails desktop stub.
  Exit criteria (empty-review flow end to end, CI green with network
  disabled) pass locally. CI not yet run on GitHub.
- Branch: `master`. Remote: `origin`
  (`https://github.com/sportifseif5-coder/loka---reviewer`).
- Push requires stripping the injected credential helper:
  `env -u GIT_CONFIG_COUNT -u GIT_CONFIG_KEY_0 -u GIT_CONFIG_VALUE_0 -u GIT_CONFIG_KEY_1 -u GIT_CONFIG_VALUE_1 git push`.

### What was built

- Core module (`go.mod`, Go 1.25, deps `gopkg.in/yaml.v3`,
  `modernc.org/sqlite`): packages `internal/config`, `internal/model`,
  `internal/store`, `internal/review`, `internal/version`,
  `cmd/loka/main.go` (`review` / `version` subcommands).
- Config loader: defaults, YAML parse with unknown-key warnings, layered
  inheritance (`LoadForRepo`), and a **presence-aware merge** fixing the bug
  where a partial file (e.g. `mode:` only) clobbered inherited scopes
  (`Merge(overlay, present map[string]bool)`).
- SQLite store: migrations v1 (reviews/findings), v2 (index tables), v3
  (learnings); `SaveReview` / `GetReview` round trip.
- Review engine: `Engine.Review`, Analyzer interface, empty-review flow with
  degradation handling, golden-file harness (`-update` flag) over
  `testdata/repos/empty`.
- Quality gates: `Makefile` (`make ci` = fmt + vet + test + offline + build
  + desktop), `.github/workflows/ci.yml` (core + desktop jobs),
  `scripts/test-offline.sh` (real `unshare -n` network isolation).
- Wails v2 desktop stub: `desktop/` with `desktop` build tag so core builds
  stay webkit-free; static frontend in `desktop/frontend/dist` checked in
  for `go:embed`; `App.Review` stub binding (uses `model.ErrEmptyRepoPath`).
  Desktop build verified locally (5 MB binary) with webkit2gtk-4.1 dev deps.

### Verified

- `make ci` green locally: gofmt clean, vet clean, unit tests pass, offline
  tests pass under real network isolation, core + desktop builds succeed.
- Presence-merge fix verified: `TestLoadForRepoInheritance` passes (user
  scope `review_tokens: 999` + repo `mode: agent` both survive).
- CLI end to end: `go run ./cmd/loka review -repo internal/review/testdata/repos/empty -db /tmp/loka-test/loka.db -json` returns an empty review result and persists it.

### Next session

1. Push to GitHub and confirm CI (core + desktop) goes green there.
2. Install **codegraph** now that source exists.
3. Start **Phase 1**: VCS adapter, indexer, analyzers, rules, provider layer,
   real engine wiring into the desktop `Review` binding.

## Session 1 - 2026-08-27: Design phase

### State

- Phase: **Pre-Phase-0 (design only)**. No source code written yet. The
  design contract is complete and is the current source of truth.
- Branch: `master`. Remote: `origin`
  (`https://github.com/sportifseif5-coder/loka---reviewer`).

### What was decided

- Product: local-first AI code reviewer ("Loka"), a desktop app that reviews
  code offline and can optionally escalate to remote LLMs and web-capable
  agents with explicit per-repository consent.
- Market position: do not compete head-on with cloud reviewers; the wedge is
  offline / air-gapped / privacy-first (regulated-industry segment).
- Foundational ADRs: Go core (0001), Wails v2 desktop shell (0002),
  provider-plugin LLM layer with local-first routing (0003), lightweight
  graph index with no embeddings at v1 (0004), SQLite + plain-file storage
  (0005), evidence-linked findings model (0006).

### Files created

- `docs/CONSTITUTION.md` - mission, values, invariants I1-I8, design
  doctrine (CodeRabbit / Greptile / Kodus), ADR summary, scope, governance,
  quality bar.
- `docs/ARCHITECTURE.md` - components, review lifecycle, execution modes,
  indexer, analyzers, provider layer, agent orchestrator, rules, learnings,
  storage, config, security model.
- `docs/ROADMAP.md` - Phase 0 (foundations) through Phase 3 (hardening),
  each with exit criteria.
- `docs/adr/` - ADR-0001 through ADR-0006 plus index.
- `AGENTS.md` - session on-boarding ritual for AI agents.

### Open decisions / confirmations

- Codename "Loka" is a placeholder; user has not requested a change.
- ADR-0002 assumes Wails v2 desktop shell; Fyne remains an alternative if
  the user prefers pure-Go native.

### Next session

1. Start **Phase 0 (Foundations)** per `docs/ROADMAP.md`: Go module layout,
   CI, storage schema, config loader, Wails shell bootstrap.
2. Install **codegraph** (agreed with user) as the navigation tool once
   there is source code to index.
3. Do not start Phase 1 until Phase 0 exit criteria pass
   (empty-review flow end to end, CI green with network disabled).
