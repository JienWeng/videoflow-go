# Guided creative workflow verification — 2026-11-17

Implemented remembered floating assistant height with pointer/keyboard resizing; dedicated character pages with editable details, AI development, reference upload, regeneration, full-image viewing and review; project visual-style selection; chat story-and-reference generation, direct image proposals, current-record next steps and canvas focus.

Chat now calls the complete generation pipeline after explicit approval. Local story generation creates/reuses cast references and identified assets, pauses for reference review, then allows storyboarding. Both local and remote guided requests stop before video submission. Rich character, asset and project-style context is included in the selected AI session. Existing native account/review gates remain intact.

## Verified

- Full Go suite passed; API suite rerun after final backend changes (including project-switch guard).
- Svelte check: 0 errors, 0 warnings. Backend/frontend production builds passed.
- Pure window tests: 24 assertions. Guidance/canvas routing: 8 tests passed.
- Real-router desktop/mobile workflow: 18 checks and 6 axe scans, no violations or browser errors. Covers style save, explicit story proposal, automatic character references, canvas focus, full-image review, rejected-reference replacement, regeneration, restoration, approved continuation and standalone image generation. No video calls before approval.
- Floating/docked/mobile window regression: 21 checks and 3 axe scans, no mutations or browser errors.
- Chat-state regression: 20 checks, no browser errors; covers send/retry, history scrolling, checkpoints, explicit proposals, specialist cancellation, stale-context repair and native review gates.
- Source review resolved remote automatic-render risk, missing remote image-review status, rejected superseded candidates blocking continuation and premature busy-state reset.
- Fixed low-contrast shared trace descriptions found by axe; no accessibility findings suppressed.
- Deployment health verified locally and through Tailscale. Readonly production browser loaded the canvas and next-step suggestions without console errors.

## Limits

Generation tests use an isolated database and fake native/image/video providers. No live paid generation was performed; real model image quality and full-body composition remain unverified. Generation prompts request complete bodies with margins; displaying existing images without cropping cannot recover content missing from their original files.

Native image invocations remain isolated and recorded in the selected application session; they do not resume the text conversation process. Projects saves visual-style fields; per-stage preset guidance remains selected in Create story. Existing typography and visual identity were preserved; no font suppression was added.
