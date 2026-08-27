# ADR-0005: SQLite + plain-file local storage

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION I5, section 11

## Context

All state must be local, open, portable, and user-owned. The user must be able
to delete the application and keep their data.

## Decision

Use SQLite for the graph index, findings, history, and learnings; use plain
Markdown/YAML/JSON files for rules and configuration. Everything lives under
the user's data directory (`~/.loka/`).

## Consequences

- No network storage; invariant I5 is directly satisfied.
- Open formats allow the user to export, edit, and migrate data freely.
- SQLite provides transactional integrity for concurrent indexer/analyzer
  writes.
- Trade-off: not horizontally scalable; irrelevant for a single-user desktop
  tool and for any later self-hosted control plane.
