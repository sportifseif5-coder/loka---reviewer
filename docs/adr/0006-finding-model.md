# ADR-0006: Evidence-linked findings model

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION I8, value 5, value 6

## Context

Review output must be explainable and trustworthy. Findings that are bare
strings cannot be verified, deduplicated, ranked, or acted upon.

## Decision

Model a **finding** as structured data: location (file, line/range, hunk),
category, severity, source (analyzer | rule | llm | agent), and reasoning
(with evidence refs). A review is a collection of findings plus a structured
summary. Findings render as inline comments in the desktop UI and, later, as
emitted comments on Git platforms (emit-only, never merge).

## Consequences

- Satisfies invariant I8: a finding always carries location and reason.
- Enables deterministic deduplication and ranking (architecture section 4.3).
- Enables the technical-debt tracker: unimplemented findings persist and can
  be revisited (Kodus doctrine).
- Trade-off: more upfront schema work; gained by verifiable, filterable,
  audit-friendly output.
