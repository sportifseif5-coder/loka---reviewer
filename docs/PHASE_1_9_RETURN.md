# Return to Phase 1.9 - Local-model dogfood

> Debt note, Session 14 (2026-11-01). Phase 2 started at the user's request
> while this Phase 1 exit criterion is still open. **You must come back here
> and finish 1.9 before calling Phase 1 complete.** Do not treat Phase 2
> progress as a substitute. Status checkboxes live only in `docs/PROGRESS.md`.

## Why this is still open

Phase 1.9 has three exit criteria. Two already passed on this repository
(noise budget, I7 human-apply). The remaining one is:

> Offline review of this repository **with a local model** produces verified
> findings in under 60 seconds.

On 2026-10-26, `make dogfood` ran this working tree in 2353ms, mode offline,
risk none, 0 findings, analyzers `secret-detection,go-vet,rules`. The LLM
stage did not run. The degradation was:

```
LLM stage skipped: no provider available
```

Nothing was listening on `localhost:11434` (Ollama) or `localhost:8080`
(llama.cpp). The deterministic baseline already meets the wall-clock budget;
the local-model half does not.

## What is incomplete (do all of this)

1. **Reachable local model.** Start Ollama (`:11434`) or llama.cpp server
   (`:8080`) on the same machine as the review. Default config already
   registers `ollama` / `qwen2.5-coder:7b`. A different local model is fine
   if `providers` in `.loka/review.yaml` or user config names it.
2. **Confirm ping.** Loka treats the provider as available when
   `GET {baseURL}/v1/models` returns any status below 500 (see
   `internal/provider/client.go`). Check that before reviewing:
   - Ollama: `curl -sS http://127.0.0.1:11434/v1/models`
   - llama.cpp: `curl -sS http://127.0.0.1:8080/v1/models`
3. **Re-run dogfood on this repository.** From the repo root:
   ```
   make dogfood
   ```
   That builds `./cmd/loka`, reviews `$PWD` into `$TMPDIR/loka-dogfood.json`,
   and prints wall-clock plus findings. Keep the JSON; it is the evidence.
4. **Prove the LLM stage ran.** Success is not "dogfood exits 0". The
   previous run already did that on the deterministic path. Required:
   - `elapsed_ms` is under 60000.
   - `analyzers_run` includes an `llm:<model>` entry (engine appends this
     only after a successful review-agent completion).
   - stderr / `degradations` must **not** contain
     `LLM stage skipped: no provider available`.
   - Dropped/demoted LLM findings may still appear as degradations; that is
     allowed. A skip of the whole stage is not.
5. **Verified findings.** Review-agent JSON is parsed in
   `internal/agent/parse.go` (file + line + message required). The
   verification agent keeps only findings on added lines; others are
   demoted, not deleted. Record whether any LLM finding survived
   verification. Zero surviving findings is acceptable **only if** the
   model actually ran and verification demoted or dropped them. If the
   model never ran, 1.9 is still open.
6. **Tick the checklist.** In `docs/PROGRESS.md` section 1.9, change the
   local-model line from partial to done, date it, paste wall-clock,
   `analyzers_run`, finding counts, and the JSON path. Update
   **Last updated**. Add a Session Notes entry. Do not start claiming
   Phase 1 COMPLETE until that line and the Phase 1 exit-criteria line
   both match reality.
7. **Do not start calling Phase 1 done from Phase 2 work.** Keyring,
   web agents, learnings, Fix Agent, and Python/TS lint bridges do not
   close 1.9.

## What is already done (do not redo)

- Shared `review.NewPipeline` so CLI and desktop both register the LLM
  router. Desktop no longer skips LLM when a model is up.
- `scripts/dogfood.sh` / `make dogfood` harness (wall-clock, risk,
  findings, degradations).
- Noise budget on this repo: 0 findings after excluding CLI/tests from
  `debug-print-in-go` and skipping tagged-out `go vet` packages.
- I7: workbench has no apply control; engine has no apply/merge/push.
- Phase 1.1 `VCSAdapter.ReadAtHEAD` and Phase 1.3 ADR-0007 (`go/parser`,
  no tree-sitter at v1).

## Environment that blocked Session 14 (2026-11-01)

Recorded so the return session does not waste time rediscovering this:

- Host: Linux x86_64, 2 CPUs, ~7.8 GiB host RAM, **no swap**.
- Cgroup `memory.current` was about 300 MiB with almost no headroom.
  A 7B (or even 1B) GGUF cannot load here without raising the memory
  limit.
- `ollama` and `llama.cpp` binaries were not installed.
- `curl` to `:11434` and `:8080` failed immediately (connection refused).
- Do not fake a model with a fixture provider and tick 1.9. Fixtures are
  the CI harness (I4). 1.9 requires a real local runtime.

If the return session is in the same constrained environment: raise the
cgroup memory limit and/or run Ollama/llama.cpp on the host, then point
`providers[].base_url` at it. Do not lower the 1.9 bar to "fixture
replayed."

## Code map for the return session

| Piece | Path |
|-------|------|
| Dogfood script | `scripts/dogfood.sh` |
| Make target | `Makefile` (`dogfood`) |
| Default local provider | `internal/config/config.go` `Default()` -> ollama / qwen2.5-coder:7b |
| Ollama / llama.cpp adapters | `internal/provider/local.go` (`:11434`, `:8080`) |
| Ping / chat client | `internal/provider/client.go` |
| Router (local first, offline excludes remote) | `internal/provider/provider.go` |
| Pipeline wiring | `internal/review/pipeline.go` |
| LLM stage + degradation text | `internal/review/engine.go` `llmStage` |
| Review agent + JSON parse | `internal/agent/review.go`, `internal/agent/parse.go` |
| Added-line verification | `internal/agent/verify.go` |
| Checklist of record | `docs/PROGRESS.md` section 1.9 |

## Invariants that still apply

- I1 / I2: local model only; no remote fallback to close 1.9.
- I3: deterministic baseline must still ship if the model dies mid-review.
- I6: stay in offline mode for this run (`config.Default().Mode`).
- I7: do not apply, merge, or push from the review tool.
- I8: accept only findings with location and message.

## Suggested return session order

1. Confirm memory is enough to load a small local model.
2. Start Ollama or llama.cpp; pass the `/v1/models` ping.
3. `make dogfood` from the repo root on a `feat/*` branch.
4. Inspect `$TMPDIR/loka-dogfood.json` for `analyzers_run` and
   degradations.
5. Update `docs/PROGRESS.md` and `docs/SESSION_NOTES.md`.
6. Conventional commit; push the `feat/*` branch (not `master`).
