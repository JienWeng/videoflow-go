# Workspace and Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship the approved canvas-first workspace and consolidate scattered Settings.
**Architecture:** Reuse the existing graph, NodePanel, ChatPanel and director APIs. Move details into a shared contextual workbench; simplify shell navigation and present settings as grouped sections, preserving underlying save handlers.
**Tech Stack:** Existing Svelte 5/SvelteKit/Tailwind/xyflow. No new dependencies.
**Spec:** docs/superpowers/specs/2026-11-11-canvas-workspace-redesign.md

## Global Constraints

- Preserve existing APIs, URLs, native sessions, paid confirmation and identity review boundaries.
- No new app dependencies.
- Mobile has focused Canvas/Workbench views; both themes and keyboard navigation supported.
- Settings extension approved by user: reduce tabs, group controls, standardize presentation.

## Review Focus

- Dirty selection changes must not put chat and editor on different nodes.
- Navigation drawers and contextual tabs must remain keyboard accessible.
- Missing native CLI/backend must explain recovery without fabricated readiness.
- Existing settings hash links must still select the intended section.
- Existing paid actions must never run from example chips or navigation.

### Task 1: Navigation and Settings

Files: +layout.svelte, settings/+page.svelte, app.css; scripts/e2e_workspace.mjs.
- [x] Add browser assertions for compact mobile navigation and grouped settings; observe failure.
- [x] Replace sprawling mobile link strip with accessible drawer; compact desktop into Workspace/Library/Tools.
- [x] Replace five Settings tabs with General, AI connections, Advanced grouping and preserve legacy hash mapping. Put defaults/appearance together and providers/engines together; routing/recovery advanced. Retain original handlers and precise paid-test labels.
- [x] Run browser navigation/settings checks and type checker.

### Task 2: Contextual workspace

Files: +page.svelte, EntityCanvas.svelte, NodePanel.svelte, ChatPanel.svelte; new shell/SessionSelector.svelte.
- [x] Add populated browser checks for contextual workbench, prompt scope, dirty guard and session selector; observe failure.
- [x] Reuse NodePanel in a single right workbench, guard selection/route changes, expose current context without overwriting chat drafts; example chips fill only.
- [x] Add inline native session selection/creation using current director APIs, explicit missing CLI messages.
- [x] Implement mobile focus switch, readable empty state and next-step links; keep chat mounted across tab switches.
- [x] Run populated checks plus component suite, type check/build.

### Task 3: Review and delivery

- [x] Independent whole-change review; fix important findings once and rerun affected checks.
- [x] Record limits, update audit/docs and commit/push to authorized main.
- [x] Refresh running Tailscale preview and verify UI/API.

Execution ruling: proceed inline under existing user approval; no additional plan approval requested because the user authorized implementation and explicitly added Settings scope.

Verification: 36 final component checks, 18 populated canvas checks, two workspace viewport checks and two session-isolation regressions passed. Full Go race suite, vet, build and Svelte check/production build passed. One independent review's Important findings fixed and regression-tested; no second review round.
