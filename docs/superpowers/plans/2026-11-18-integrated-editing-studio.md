# Integrated Editing Studio Implementation Plan

**Goal:** Deliver the five approved studio priorities through shared working features, without paid generation during verification.
**Architecture:** Reuse the composition manifest, local-session planner, ChatPanel, source ownership checks, provider adapters and local renderer. One shared editing workspace owns draft/selection/playhead/undo. AI changes remain explicit proposals. Finishing and localization return editable cues; generation references are distinct from style extraction.
**Tech stack:** Go, Svelte 5, existing HyperFrames/FFmpeg and fal Seedance reference endpoint.

- [x] STUDIO-01 Extract a reusable editing workspace and embed it in Studio and the existing composition route. Share preview, selection, draggable/snapping tracks, inspector, draft recovery and undo. Keep advanced fields collapsed.
- [x] STUDIO-02 Use the main ChatPanel for local-session edit proposals. Preserve the current edit until Apply, display a diff, reject stale proposals and allow undo; validate available sources and linked-caption invariants.
- [x] STUDIO-03 Add one finishing panel for transcription from actual clip audio, editable captions, soundtrack selection and explicit save/export. Retain source timings and avoid destructive automatic replacement.
- [x] STUDIO-04 Add explicit reference-driven video generation for the officially supported fal endpoint, with image/video/audio roles, validated limits, a current read-only plan, confirmed generation and duplicate/uncertain-request fences. Unsupported providers stay unavailable.
- [x] STUDIO-05 Add reusable/importable edit layouts, project brand settings, and editable caption-language variants with stable IDs/timings. Do not claim generated dubbing, avatars or full-video translation from subtitle translation.
- [x] STUDIO-06 Verify each slice with targeted tests, then full Go/type/build and mocked desktop/mobile/accessibility flows. Update PR #2, preserve auth/data, deploy after checking no active generation.

Scope: these implement the five approved next steps. Avatar creation, team collaboration, arbitrary speed/keyframe effects and production-quality guarantees for untested provider outputs remain separate capabilities, not implied by this delivery.

Verification: full Go suite passed; Svelte zero errors/warnings; production Docker builds passed. Twenty desktop/mobile acceptance checks, two zero-violation WCAG scans and zero browser errors; earlier chat and creative-brief browser regressions pass. Actual local HyperFrames lint/snapshot/export with a title card passed. No paid provider inference or generation was submitted. Details and boundaries: docs/INTEGRATED-EDITING-STUDIO.md.
