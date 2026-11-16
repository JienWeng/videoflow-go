# Scene studios and shared asset consistency — 2026-11-16

## Implemented workflow

The existing scene workspace is now named Scene studios. Canvas scene actions open the same workspace; selected scene URLs survive reload, and mobile deep links open that scene's details. The header keeps the scene name visible. Progressive disclosure and dirty scene/shot drafts remain intact.

Each scene has a reference workspace showing its cast's current reference images, selected props/locations, review states and identity locks. Props and locations are shared project Library IDs. Explicit selection/removal uses scene revisions and atomic updates to both conditioning bindings and graph associations. Same-group version replacement removes the old binding and adds the new one in one transaction. Legacy attached props are recognized. Scene Library filtering follows actual scene/cast associations as well as generated image history, rather than origin metadata alone.

The Library supports manual name, description and identity editing without an AI call, with optimistic concurrency. Appearance changes require renewed approval. Legacy lock editing and vision recognition preserve that review invariant. Usage lists identify scene, character, shot, render, session and version dependencies; in-use assets cannot be deleted. Foreign-project reads/deletes and reference mutations are rejected. Deletion and version generation share a lock so a source cannot disappear during generation.

Regenerating a prop or location creates a pending candidate in its version group, preserving source locks and the old scene bindings. Source history remains available. A candidate is only used in an existing scene when explicitly selected/replaced, and cannot condition generation until approved. Native Codex-first routing and the existing fallback policy are retained.

A scene's generation context is its saved cast and references, not incidental chat selection. A prop reference cannot substitute for a video keyframe. The preview and render use the newest reviewed matching storyboard; pending candidates remain directly visible for review. Separate scene input revisions cover scene content/specification, shots and visual style. Old images become stale after those inputs change; explicit current-scene approval can attest an existing image, or the user can regenerate it. Queued renders recheck their frozen scene input revision before submission.

Nonpersistent style pinning was removed in favor of explicit, charge-disclosed style extraction from an image. Partial project style edits now retain omitted fields and update the existing project style rather than creating competing rows.

## Verification

- Full Go suite passed, including nine new scene/asset tests covering canonical scope, prop/keyframe separation, shared reuse, CAS, project ownership, rejected images, safe deletion, asset versions, partial style edits, manual asset identity editing, newest keyframe selection, content freshness, explicit review and queued render fencing.
- Svelte check: zero errors and warnings. Production frontend and backend images built successfully.
- 20 desktop/mobile browser checks against the real isolated Go router plus fake native CLI/image provider; eight axe scans passed. Tests edit and approve a shared prop, preserve a typed scene draft during reference changes, generate one candidate version, replace it in only one scene, regenerate/review a matching storyboard, and verify no video calls and exactly three fake image submissions per viewport. Evidence: `2026-11-16-scene-studio-assets-results.json`.
- 48 broader production-frontend workspace checks and six axe scans passed, covering canvas/chat, Library navigation, stage recall, mobile behavior, error recovery and draft preservation. Evidence: `2026-11-16-scene-studio-workspace-results.json`.
- 22 progress assertions and two canvas layout tests passed.
- Independent code review and follow-up found no remaining important defects in the implemented scope.

Testing found and fixed mobile scene deep links opening the list instead of details and storyboard selection using an older image. Existing regression fixtures were updated for the Scene studios label, direct-link mobile behavior and new read-only reference/usage endpoints. One browser run closed unexpectedly; its result was discarded and the full suite rerun successfully against a stable production build.

## Limits

Tests use isolated data and fake providers; no live image/video generation, story retry, credential change or native-session acknowledgement was performed. The selected production Codex session remains review_required, so account-level native image generation is still unverified. Visual correctness still requires reviewing actual generated images.

Deletion protects current bindings, version provenance and frozen render requests. Detached historical storyboard snapshots retain descriptive provenance; they do not independently pin every source file forever. Existing images, previous versions and user data were not deleted or automatically replaced.
