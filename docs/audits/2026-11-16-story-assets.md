# Automatic story assets verification — 2026-11-16

## Implemented behavior

Local Create story plans essential props and locations against the current project library after planning shots. The planner receives names, descriptions, metadata/locked traits and exact library IDs. Proposed IDs must remain available, unrejected and of the requested type. Generation rechecks the library under a project lock; exact type/name/description matches reuse one image across scenes. Existing asset metadata is never rewritten.

The existing reference-review checkpoint now includes characters, props and locations, reuse/generation labels and reasons. Removing an unsuitable prop/location only detaches it from this story; a database transaction fences concurrent continuation and retains the library file. Approval, availability and the combined eight-image context limit are checked before claiming continuation. Current scene bindings condition local storyboards and video identity snapshots; the legacy reference resolver includes scene props too. Video rendering remains explicit.

Manual scene asset suggestions and generation share reuse/materialization behavior. Explicitly selected plan items are submitted intact rather than converted to prose and replanned. Scene endpoints check active-project ownership.

## Verification

- Full Go suite passed with the repository test image including FFmpeg. Latest API suite passed after final error-path changes.
- Focused tests: appearance-sensitive matching, provider-free reuse, deduplicated links, metadata preservation, invented/foreign ID rejection, transactional removal, stale continuation fencing, missing image generated only once across scenes, and real guided pipeline propagation of an approved reused prop alongside two actor references.
- Svelte check: zero errors or warnings.
- Progress: 18 existing assertions plus four new automatic-assets assertions. Two canvas-layout tests passed.
- Production frontend browser fixture: 22 checks across desktop/mobile and four axe accessibility scans passed. Evidence: `2026-11-16-story-assets-results.json`.
- Production-build workspace regression: 48 browser checks and six accessibility scans passed; Studio, Library, floating assistant, stage recall and draft preservation. Evidence: `2026-11-16-story-assets-workspace-results.json`.
- Existing character-review regression: 20 browser checks and four accessibility scans passed.
- Independent read-only code review found missing legacy reference propagation; fixed in `sceneReferences`.

An initial browser attempt raced Vite dependency/module reload and was discarded. The new asset-review suite was rerun successfully against the production frontend image. All browser writes and generation requests were intercepted by fixtures. No live paid image/video generation, story retry, credential change or session-review acknowledgement was made.

## Limits

Semantic suitability is a model proposal and remains subject to the user's reference review. Exact server deduplication intentionally requires matching appearance descriptions; a shared name alone is insufficient. This change applies automatic planning to new local story runs, and does not restart failed or existing runs.
