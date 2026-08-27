# ADR-0004: Lightweight graph index, no embeddings at v1

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION section 7, scope section 9

## Context

Cross-file understanding is required for meaningful review (impact beyond the
diff, agentic exploration), but resource cost must stay low for an offline
desktop tool.

## Decision

Build a lightweight graph index: files, symbols, references (calls/imports),
and external dependencies, stored in SQLite and updated incrementally. Do not
build vector embeddings or semantic RAG at v1.

## Consequences

- Provides `ImpactSet(diff)`, caller/callee queries, and dependency usage
  sites with low memory and disk footprint.
- Fast incremental updates on a laptop-class machine.
- Semantic search (embeddings) remains a deferred enhancement behind the
  index query interface, so it can be added without changing consumers.
- Trade-off: weaker fuzzy/semantic recall than RAG; acceptable because the
  deterministic pipeline and rules carry the primary signal at v1.
