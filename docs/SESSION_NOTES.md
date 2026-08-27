# Session Notes - Loka

> Companion to `AGENTS.md` section 2. Records what was done and what is next.
> Updated at the end of every session. History is authoritative; this file is
> the summary.

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
