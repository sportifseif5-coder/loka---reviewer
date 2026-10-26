# ADR-0007: Pure-Go parser at v1, not tree-sitter

- **Status:** Accepted
- **Date:** 2026-10-26
- **Related:** CONSTITUTION section 7, ROADMAP Phase 1.3, ADR-0001, ADR-0004

## Context

Phase 1.3 planned tree-sitter syntax checks as part of the deterministic
analyzer pipeline. The indexer already parses Go with the standard library
(`go/parser` + `go/ast`) so indexing stays offline, dependency-free, and
bit-for-bit deterministic (invariants I2/I3). Adding tree-sitter would
introduce a C/native runtime, extra binaries, and a second syntax tree for
the same Go sources.

## Decision

Do not add tree-sitter at v1. Keep `go/parser` as the Go syntax backend.
Revisit tree-sitter only if a non-Go syntax tier is required (Python and
TypeScript lint bridges are Phase 2).

## Consequences

- No new native dependency; `make ci` and the netns offline suite stay
  fully static-Go.
- Go syntax errors surface through `go vet` and indexer skip notes rather
  than a dedicated tree-sitter analyzer.
- A later non-Go syntax tier can still plug in behind the existing
  per-file symbol extraction seam (ADR-0004) without changing storage or
  query surfaces.
