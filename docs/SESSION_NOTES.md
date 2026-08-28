# Session Notes - Loka

> Companion to `AGENTS.md` section 2. Records what was done and what is next.
> Updated at the end of every session. History is authoritative; this file is
> the summary.

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
