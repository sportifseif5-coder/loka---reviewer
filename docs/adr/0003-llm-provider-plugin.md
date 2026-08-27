# ADR-0003: Provider-plugin LLM layer

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION I4, I6, section 8

## Context

The product must work offline with local models and online with remote
models, using user-owned keys, with no vendor lock-in and no bundled vendor
SDKs.

## Decision

Introduce a `Provider` interface (`Name`, `Available`, `Complete`). Ship local
providers (Ollama, llama.cpp) and remote OpenAI-compatible providers. The core
engine never imports a vendor-specific SDK.

## Consequences

- Offline reviews run against a local model with no keys and no network.
- Remote providers are configured per repository with BYO keys, read from the
  OS keyring or a user-owned 0600 config file.
- Routing policy: local preferred; remote failure fails over to the next
  configured provider, then to "LLM skipped" with the deterministic baseline
  still delivered.
- A recorded-fixture adapter enables offline, keyless CI testing of the whole
  pipeline (constitution section 10.6).
- Trade-off: cannot use vendor-exclusive features (e.g. tool-use quirks);
  mitigated because agents operate over the provider's plain
  completion/chat interface.
