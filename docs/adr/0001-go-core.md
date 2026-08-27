# ADR-0001: Go core engine

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION section 7

## Context

The reviewer must be a cross-platform offline desktop application with fast
codebase indexing, a deterministic analyzer pipeline, and a parallel
LLM/agent orchestration layer. Distribution must be a simple, self-contained
binary with no runtime or dependency download.

## Decision

Implement the core engine in Go.

## Consequences

- Single static cross-platform binary; trivial offline distribution.
- Excellent concurrency primitives (goroutines, channels) for parallel
  analyzers and agent swarms.
- Fast incremental indexing with low memory footprint.
- Good tree-sitter bindings and a large analyzer ecosystem.
- Trade-off: smaller ML/LLM library ecosystem than Python, mitigated by the
  provider-plugin seam (ADR-0003) which speaks plain HTTP/OpenAI-compatible
  protocols.
