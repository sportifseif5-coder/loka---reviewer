# AGENTS.md - Session Protocol for AI Agents

This file is the mandatory on-boarding ritual for any AI coding agent
(starting a new session) working in this repository. Follow it before any
task, edit, test, or build.

## 1. Load the design contract first

Read, in this order, before doing anything else:

1. `docs/CONSTITUTION.md` - the non-negotiable contract. Invariants I1-I8
   and the design doctrine are binding. A change that conflicts with an
   invariant is rejected, not debated.
2. `docs/ARCHITECTURE.md` - component boundaries, data flow, execution
   modes, interfaces.
3. `docs/adr/README.md` and the ADRs it references - binding technical
   decisions.
4. `docs/ROADMAP.md` - current phase and its exit criteria.
5. `docs/PROGRESS.md` - the living phase/task/step checklist. It is the
   checklist of record; keep it in sync with reality (section 4).

## 2. Recover the session state

1. Read `docs/SESSION_NOTES.md` and `docs/PROGRESS.md` - they record what was
   done and what is next.
2. Run `git log --oneline -10` and `git status` to confirm where the repo
   actually stands versus what the notes claim.
3. If `docs/SESSION_NOTES.md` says a phase is in progress, resume it. Do not
   start a new phase unless the current phase's exit criteria pass.

## 3. Follow the constitution during work

- Offline-first: never make a capability depend on the network.
- No code exfiltration: nothing leaves the machine unless a provider/agent
  is explicitly enabled for the repository.
- Deterministic baseline first: analyzers and rules are ground truth; LLM
  output is labeled as non-deterministic.
- Human-in-the-loop: Loka never auto-applies, auto-merges, or auto-pushes.
- The reviewer must review its own changes (dogfooding).

## 4. Update state at session end

Before finishing a session:

1. Update `docs/PROGRESS.md`: tick every finished step/task and update its
   **Last updated** line. This is mandatory on every step completion, not
   only at session end.
2. Update `docs/SESSION_NOTES.md` with what changed and what is next.
3. Run the quality gates (see ROADMAP / constitution section 11).
4. Commit with a Conventional Commits message.
5. Push to the configured remote. **A session that does not push leaves the
   work at risk of loss. Pushing is not optional.**

## 5. Tooling notes

- Codegraph (if installed) is the navigation tool for this codebase; its
  output directory `.codegraph/` is gitignored and never committed.
- `make status-source` enforces that work-status markers live only in
  `docs/PROGRESS.md`; `make progress-hint` (run by `make ci`) warns when Go
  changes lack a `docs/PROGRESS.md` update.
- The repo's own review tool, when it exists, must be run on this
  repository's own pull requests.
