# Application UX fixes Implementation Plan

> **For agentic workers:** Use executing-plans inline, with one final fresh review. Preserve earlier workspace changes.

**Goal:** Resolve the verified whole-application audit findings and deliver a tested PR.
**Architecture:** Keep Svelte/Go patterns. Use semantic keyboard controls, existing confirmation behavior, and a project-scoped filesystem composition index with no dependency additions.
**Tech Stack:** Svelte 5/SvelteKit, Go, SQLite/filesystem storage, Docker, Playwright fixture workflows.
**Spec:** docs/superpowers/specs/2026-11-12-application-ux-fixes.md

## Global Constraints
- Preserve the product’s Inter typography, monochrome palette and existing canvas kind cues.
- No real credential mutation, paid generation, destructive live-data action or merge.
- Preserve existing uncommitted user work; one feature branch/PR includes the accumulated interface improvements.
- Unknown checks remain explicitly unverified until evidence exists.

## Review Focus
- Dirty-state snapshots must not change on failed save or unrelated refresh; project switching and reload require protection.
- Composition revision listing must be project scoped, path-safe and not expose other projects’ records.
- Keyboard activation must not also trigger editor playback/seek shortcuts or pointer drag.
- Native form validation and accessible error feedback must agree with backend limits.
- Off-Studio new-story intent must survive navigation without auto-submitting a provider request.

## Tasks
- [ ] Add failing browser/API regression coverage for the reproduced issues.
- [ ] Fix editor keyboard/mobile controls, palette navigation/semantics, character error states, sender labels and route titles.
- [ ] Guard credential, voice and composition drafts; align Settings fields/loading/readability and semantic aliases.
- [ ] Add a project-scoped saved-composition index and reopen entry, plus upload constraint validation/guidance.
- [ ] Run type/build/backend and relevant desktop/mobile browser workflows; inspect rendered changes.
- [ ] Obtain fresh whole-change review, resolve important findings, update audit closure notes, commit/push and create/link PR.

## Decisions
- User approval of the audit fixes supplies the concrete design direction; no additional design confirmation is required.
- Work on the current checkout’s feature branch so accumulated uncommitted changes remain intact.
