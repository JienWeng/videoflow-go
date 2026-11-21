# Unified Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** One Studio workspace with shared creation/library editors and reversible stage disclosure.
**Architecture:** Extract existing route bodies into reusable components. Root Studio coordinates preserved Canvas/Story views. Global creation dialog uses a small Svelte store. Scene stage visibility derives from real saved content.
**Tech Stack:** Svelte 5, SvelteKit, existing shadcn-svelte/Bits UI, Svelte Flow.
**Spec:** docs/superpowers/specs/2026-11-16-unified-workspace-design.md

## Global Constraints
- Preserve deep links, drafts, history, approval and provider gates.
- No new dependencies, paid calls or visual identity replacement.
- Controls must remain labelled, keyboard accessible and usable at 390px.

## Review Focus
- Switching views/tabs must preserve unsaved edits.
- Empty or incomplete scenes must not expose paid future actions as ready.
- Existing late-stage work must remain reachable despite missing earlier stages.
- Opening/closing creation must never submit or replay an operation.
- Existing deep links, error recovery and mobile navigation must work.

### Task 1: Unified shell and shared flows
Files: routes/+layout.svelte, routes/+page.svelte, routes/{create,characters,assets,scenes,library}/+page.svelte; lib/{create,library,scenes,shell}; scripts/e2e_unified_workspace.mjs.
- [x] Write browser contract for canvas default, summary menu, library tabs, shared Create story, preserved drafts and routes; run against old app and record failure.
- [x] Extract existing editors with embedded/title options. Add common creator dialog with dirty-close protection, preserve /create full-page route. Remove the alternate scene script-generation form in favor of the shared dialog.
- [x] Make Canvas/Story root views; preserve mounted work across switches, put summary in project menu, replace duplicate nav destinations with Library and conversation actions. Keep advanced editors accessible.
- [x] Test browser contracts at desktop/mobile including no unexpected mutations and errors.

### Task 2: Stage disclosure and selected-item tools
Files: lib/scenes/stages.ts, stages.test.mjs, SceneDetail.svelte; lib/canvas/EntityCanvas.svelte, NodePanel.svelte.
- [x] Write and run failing stage tests for empty/partial/completed/out-of-order work.
- [x] Implement reached-stage visibility and current-stage default-open accordions, reversible Show all stages, saved-stage recall and contextual selected-item toolbar. Preserve mutations and approval checks.
- [x] Verify stage review/collapse/view switches never submit requests, edits survive hiding, and reference/render links remain explicit.

### Task 3: Verification and delivery
- [x] Run Svelte check/build, unit tests and focused mocked browser flows plus accessibility scans.
- [x] Obtain independent whole-change review and address important issues.
- [x] Update audit evidence, deploy local frontend, verify HTTP/browser, commit/push to PR #2 and register PR.
