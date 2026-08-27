# ADR-0002: Desktop app shell (Wails v2)

- **Status:** Accepted
- **Date:** 2026-08-27
- **Related:** CONSTITUTION section 7, section 9

## Context

v1 must ship as a desktop application ("desktop app"), not a web service or
CLI-only tool. The frontend needs a rich review workbench: diff viewer,
findings list, rule editor, and history browser.

## Decision

Use Wails v2: a Go backend with the system webview rendering a web-tech
frontend, packaged as a native desktop app.

## Alternatives considered

- **Fyne/Gio (pure Go):** no webview dependency, but weaker diff/code
  rendering and slower UI iteration for a code-focused workbench.
- **Electron + Go sidecar:** heavy distribution, larger attack surface for a
  privacy-focused tool.
- **Tauri + Rust:** excellent, but conflicts with the Go core (ADR-0001).

## Consequences

- One Go codebase serves as both core engine and desktop backend.
- The frontend reuses web technologies (diff viewers, syntax highlighting)
  inside a native window.
- Webview quirks are isolated behind the frontend layer; core engine remains
  platform-independent.
- Trade-off: depends on a webview runtime on target OSes, handled by Wails
  packaging; the engine itself (all review logic) has no GUI dependency.
