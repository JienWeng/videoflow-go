# Connected Video Implementation Plan

> Execute inline in verified slices using the incremental implementation workflow.

**Goal:** A cost-aware connected story-to-audio/video-to-music/export flow.
**Architecture:** Scene production settings and read-only plans feed immutable render units; the existing worker and local composition editor execute them. Native/saved/silent audio policies remain explicit.
**Tech Stack:** Go, SQLite, Svelte 5, existing fal/BytePlus clients and pinned HyperFrames.
**Spec:** docs/superpowers/specs/2026-11-17-connected-video-design.md

## Global constraints
- No live paid provider requests in verification; preserve native authentication, saved identities and all existing videos.
- Revalidate ownership, references, source revisions and plan choice before spending.
- No unconfirmed multi-call batch, silent fallback, automatic paid retry or fabricated exact timing.

## Checklist
- [x] FLOW-01 Native/saved/silent audio modes, native default where supported; worker retains chosen mode.
- [x] FLOW-02 Saved voice setup only blocks saved mode; native dialogue/music direction and honest capability messaging.
- [x] FLOW-03 Automatic fewest-call consecutive shot grouping and explicit per-shot mode, supported durations and trim lengths.
- [x] FLOW-04 Read-only paid-call/generation-duration/reuse plan; confirmed plan ID checked at render boundary.
- [x] FLOW-05 Frozen unit metadata, transactional batch insertion, partial retry reuse and uncertain-job fencing.
- [x] FLOW-06 Scene and chat share production settings and plan; follow-up links reach assembly and appropriate audio tools.
- [x] FLOW-07 Ordered assembly loads one output per unit, correct aspect and trims, never duplicate scene versions.
- [x] FLOW-08 Native music prompt plus uploaded/library music; gain/fade/background-level controls work in preview and export.
- [x] FLOW-09 Verify plan/audio/ownership/staleness/reuse/assembly with fixtures, real local media and mocked browser flows.
- [ ] FLOW-10 Full regression/type/build/accessibility checks, review, PR update and deployment with existing outputs preserved.

## Review focus
- Changed scene/provider/settings after plan display must reject before provider calls.
- Completed, running and uncertain units must never become unintentional new charges.
- Native mode must reach the provider as audio enabled; silent and saved modes must remain silent.
- Generated padding and alternate versions must not inflate final assembled duration.
- Music controls must affect actual local export, not only the browser preview.

## Ordered slices
1. Add production settings/plan and audio policy tests; implement server and worker contracts, verify.
2. Add atomic batch render and ordered composition draft tests; integrate production controls, verify.
3. Add music controls with local media export verification and browser regression coverage.
4. Run full checks, inspect changes and deploy through PR #2 without paid generation.

FLOW-01 through FLOW-09 are completed; FLOW-10 awaits final deployment health checks. Detailed verification and remaining model-quality limits: docs/audits/2026-11-17-connected-video-review.md.
