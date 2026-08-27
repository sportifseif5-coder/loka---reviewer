# Constitution - Loka

> Working title: **Loka** (placeholder; a "local-first code reviewer").
> This document is the foundational contract of the project. Every design
> decision, pull request, and line of code must be traceable to this document.
> When a change conflicts with an invariant, the change loses.

## 1. Preamble

We are building a **code reviewer that works fully offline and can suggest
bug fixes**. It is a desktop application that understands a local codebase,
produces review findings with actionable fixes, and never requires a network
connection to be useful. When the user explicitly enables it, the same engine
can escalate to remote LLMs and web-capable agents for deeper analysis.

The product exists because the mainstream AI code-review market is
cloud-first and saturated. Our wedge is **privacy, locality, and ownership**:
the code never has to leave the machine, and the user keeps control of every
model and every agent it runs.

## 2. Mission

> Help developers ship better code without ever forcing their code, their
> data, or their review habits into a third party's cloud.

## 3. What Loka Is and Is Not

### Is
- A desktop application (local GUI shell backed by a Go core engine).
- A deterministic-first reviewer: static analyzers, linters, tree-sitter
  parsing, and user rules run always and are reproducible.
- An LLM-augmented reviewer: a provider-plugin layer that serves local models
  (Ollama, llama.cpp) when offline and any OpenAI-compatible or provider API
  when online.
- An agent orchestrator: specialized review, verification, and (opt-in)
  web-surfacing agents work in parallel over the diff and the codebase graph.
- A learning system: it records accepted/rejected suggestions and reviewer
  feedback so future reviews match the user's standards.
- A single, self-contained, cross-platform binary distribution.

### Is not
- A cloud service. There is no mandatory backend, no telemetry that contains
  code, no account required to use core functionality.
- A general-purpose chatbot. Loka reviews code; chat is a support surface, not
  the product.
- A replacement for human review. Loka produces evidence and suggestions; the
  human always holds the merge decision.
- A CI-only tool bolted onto the web. Local review is the primary surface;
  optional Git-platform integration is a later, secondary surface.

## 4. Core Values

1. **Offline-first.** Every headline capability must have a fully working
   offline path. Network is an enhancement, never a dependency.
2. **Privacy by default.** Nothing leaves the machine unless the user
   explicitly enables a provider or agent. Local model providers are the
   default. The user's code is never used to train anything, ever.
3. **No lock-in.** Model-agnostic. OpenAI-compatible endpoints, Ollama,
   llama.cpp, local and remote. Bring-your-own-key everywhere. Rules and
   review history are plain files the user owns.
4. **Determinism before magic.** The same input and the same configuration
   must always produce the same baseline (analyzer-only) findings.
   Non-determinism is isolated to the LLM layer and labeled as such.
5. **Explainability.** Every finding must carry evidence: the file, the line,
   the rule or analyzer that produced it, and the reasoning behind it.
6. **Human-in-the-loop.** Review output is advisory. Applying fixes, merging,
   and dismissing findings are explicit user actions.
7. **Quality over speed.** Slow, correct analysis beats fast, noisy analysis.
   Noise is the number one reason AI reviewers get disabled.
8. **Composable and extensible.** Analyzers, providers, rules, agents, and
   storage are pluggable interfaces, not monoliths.

## 5. Non-Negotiable Invariants

These constraints can never be violated, even by a "good reason." Any
proposed change that breaks one must instead change the proposal.

- **I1. No code exfiltration by default.** No code, diff, or derived source
  content is transmitted outside the process unless the user has enabled a
  provider or agent for that repository.
- **I2. Full offline functionality.** With zero network and a local model
  available, every v1 user story must remain achievable.
- **I3. Deterministic baseline.** Analyzer/rules-only reviews are
  reproducible bit-for-bit given the same repo snapshot and config.
- **I4. Model-agnostic core.** The core engine never imports a specific
  vendor SDK. All LLM interaction goes through the provider interface.
- **I5. Local data ownership.** Index, learnings, rules, config, and history
  live in the user's filesystem in open formats (SQLite + plain files).
- **I6. Consent for remote.** Every network-enabled capability (remote LLM,
  web search, issue trackers) is off by default and requires explicit,
  per-repository enablement.
- **I7. Human merge authority.** Loka never auto-merges, never auto-pushes,
  and never mutates the user's working tree without an explicit user action.
- **I8. Evidence attached.** A finding without a location and a reason is a
  bug in Loka, not a valid finding.

## 6. Design Doctrine (informed by CodeRabbit, Greptile, Kodus)

We adopt the strengths of the three reference systems and reject their
lock-in and cloud assumptions.

### From CodeRabbit - orchestration and depth
- **Orchestrated agent system, not a single prompt.** Review, Verification,
  and (opt-in) Web agents run as separate roles with separate goals, sharing
  a common context store.
- **Multi-dimensional analysis.** A pipeline of 50+ static analyzers and
  linters complements the LLM. The LLM never has to "remember" to check for
  unused imports or obvious bugs; the analyzers catch those first.
- **Agentic exploration.** When an LLM needs context, it can traverse the
  graph index and read files itself, instead of only seeing the diff.
- **Living memory (Learnings).** Accepted/rejected suggestions and user
  feedback are distilled into durable review preferences.
- **Path-scoped instructions.** Rules can target globs
  (e.g. `src/controllers/**`).
- **Structured review summary.** A concise walkthrough of the change,
  its risk, and its findings - not just a pile of inline comments.
- **Finishing touches (opt-in).** After review, agents may propose applying
  fixes, docstrings, or tests - always as suggestions the user applies.

### From Greptile - codebase intelligence and swarms
- **Graph index.** A persisted per-repository graph of files, symbols,
  functions, types, and dependency edges. This is the substrate for
  cross-file impact analysis and agentic exploration.
- **Impact beyond the diff.** Reviewers must reason about what the changed
  code calls and what calls it, not only the changed lines.
- **Parallel swarm of reviewers.** Multiple specialized agents review the
  change concurrently and their findings are deduplicated and ranked.
- **Rules in plain language.** Teams/users write standards in natural
  language; Loka enforces them on every review.
- **Self-hosting / air-gap as first class.** Local-first is not a feature;
  it is the architecture. (We replace their "self-hosted cloud" with a
  true local desktop runtime.)

### From Kodus - openness, rules, and economics
- **Open core with no vendor margin.** No token resale. The user pays their
  model provider at list price, or runs local models for free.
- **Plain-language review rules** in a file the user owns
  (`review_rules.md`), plus **rule sync** from existing rule files
  (Cursor, Copilot, Claude, VS Code settings) so users keep their standards.
- **Model-agnostic providers** - any OpenAI-compatible endpoint, self-hosted
  models, local runtimes.
- **Technical-debt tracking.** Unimplemented suggestions are recorded and
  can be revisited, so insights do not vanish.
- **Private by default, no training on user code.**

## 7. Foundational Decisions (ADR summary)

Formal ADRs live in `docs/adr/`. Current binding decisions:

| ID | Decision | Rationale (short) |
|----|----------|-------------------|
| ADR-0001 | Core engine in **Go** | Single static binary, fast indexing, excellent concurrency for parallel agents, easy cross-platform offline distribution. |
| ADR-0002 | **Desktop app** shell (Wails v2: Go backend + system webview) | Meets "desktop app" target while reusing web-tech frontend; one codebase, native packaging. |
| ADR-0003 | **Provider-plugin** LLM layer | Ollama/llama.cpp when offline; any OpenAI-compatible API when online. One interface, many backends. Invariants I4/I6. |
| ADR-0004 | **Lightweight graph index** (symbol/type/import graph, no embeddings at v1) | Cross-file context with low resource cost; embeddings deferred as an optional enhancement. |
| ADR-0005 | Storage in **SQLite + plain files** | Open, portable, user-owned. Index/learnings/history all local. Invariant I5. |
| ADR-0006 | Review output model = **findings + evidence**, rendered as inline comments in the desktop UI and (later) on Git platforms | Aligns with explainability (value 5) and human-in-the-loop (value 6). |

## 8. Execution Modes

- **Offline mode.** Deterministic analyzers + local model via the provider
  plugin. No network calls, ever. All v1 workflows available.
- **Online mode (explicit).** Same engine, remote providers enabled per
  repository. Users may bring their own keys/endpoints.
- **Agent mode (explicit, per repository).** Web-capable agents are allowed
  to surface external information (docs, CVEs, best practices) and may use
  issue trackers. This is the highest-privilege mode and requires clear
  per-repository consent.

A repository's mode is stored in its local config and is visible in the UI
at all times. See `docs/ARCHITECTURE.md` section 3 for the full mode
semantics and degradation rules.

## 9. Scope Boundaries for v1

We will NOT build in v1 (explicitly deferred):

- Vector embeddings / semantic RAG over the codebase.
- Public web CI service or hosted SaaS control plane.
- Full Git-platform (GitHub/GitLab) PR posting (a thin CLI/CI adapter is a
  Phase 2 surface, and it must be a pure emitter, never a merge actor).
- Multi-user/team features, org dashboards, SSO.
- Auto-apply of fixes without a user confirmation dialog.
- Mobile or browser-only deployment.

## 10. Governance and Development Rules

1. **Constitution-first.** Every feature must cite the invariant, value, or
   ADR it serves. Features that serve none are rejected.
2. **Change the constitution deliberately.** Amending this document requires
   a written proposal (an ADR) that states what changes, why, and which
   invariants are affected. Amendments are reviewed as seriously as code.
3. **Review the reviewer.** This repository's own pull requests must pass
   Loka's own review when available, and a human review at all times.
4. **Dogfooding rule.** No user-facing feature ships until it is exercised
   on this repository's own codebase.
5. **No secret handling in code.** Keys are never committed. Providers read
   keys from the user's OS keyring or a local config file the user owns.
6. **Testing requirements.** Deterministic analyzers require golden-file
   tests. The provider interface requires a recorded-fixture test harness so
   offline CI runs without network or keys.
7. **Branch and commit conventions.** Conventional commits; feature work on
   `feat/*` branches; documentation changes on `docs/*` branches; commits
   must be small, reviewable units.

## 11. Quality Bar

- **Correctness:** golden tests for every analyzer; deterministic baseline
  must be reproducible.
- **Noise budget:** false-positive rate on a fixed benchmark corpus is a
  release gate. Regressions in precision block the release.
- **Performance:** index + review of a mid-size repo (e.g. 100k LOC, 2k
  files) must complete in interactive time on a laptop in offline mode.
- **Security:** the desktop runtime is sandboxed from the user's dotfiles;
  agents run with declared capability scopes.
- **Docs:** every public interface and config key is documented. No "doc
  debt" at release.

## 12. Success Metrics

- **User value:** median time from "open app" to "actionable first finding"
  under 60 seconds on a mid-size repo, offline.
- **Trust:** >= 90% of suggested fixes accepted by the user over a rolling
  month of usage.
- **Resilience:** zero code-path network calls in offline mode (verified by
  integration tests that run with networking disabled).
- **Ownership:** user can delete the app and keep all index/rules/history as
  portable open files.

---

*Placeholder codename "Loka" is easily changed. The constitution's substance
is the contract; the name is not.*
