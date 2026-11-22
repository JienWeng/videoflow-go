# Story creation progress — 2026-11-16

User task: understand story creation, know the current step, and review images before rendering. Preserve the existing visual identity and actual provider gates.

## Verified refinements

- Six explicit checkpoints: story, scenes, shots, storyboard images, user review, scene rendering. Server stages determine completion; no fabricated percentage or ETA.
- Current step has a short explanation, completed checkpoints use checkmarks, upcoming rendering says After approval. The review state has a prominent Review storyboard action above the timeline, visible without scrolling on mobile; per-scene review/render controls remain below.
- The request form stays hidden after storyboarding, with explicit Create another story or Edit brief & try again actions. No automatic retry or video submission.
- Connection errors retain checkpoints. Failed and reconciliation-required operations remain in the progress view, retain saved work, and expose explicit recovery. Duplicate recovery links and irrelevant render-job links are removed.
- Legacy rendering does not prove reference approval. Independent review caught this inference; saved renders now say Review not recorded instead of claiming review is done.
- Dialog header and compact Close footer remain visible; the body scrolls within the viewport. Closing submitted work does not cancel its saved backend operation.

## Verification and counts

Expected/reported **83/83 browser checks**, **8/8 accessibility scans**, **12 state assertions**; no remaining selected IDs. Every check and its evidence is recorded in `2026-11-16-story-progress-results.json`. Suites: new progress 20, workspace regression 48, creation dialogs 15. Browser API writes mocked; only the expected fake generation submission occurred in the progress suite. Approval enabled a render button without clicking it. Svelte check: zero errors/warnings. Production frontend Docker build and whitespace check passed. Independent focused review clean after the legacy-review correction.

The approval test waits for the asynchronous refresh to enable Render, rather than assuming the click synchronously completes its fetch. Native inspection checked the actual saved operation without retrying it; fixture screenshots checked the dialog at desktop/mobile widths. No new dependencies or design suppressions.

## Separate verified issue, left unresolved in this UI change

The existing dog-story operation failed at shot planning: `json: cannot unmarshal number 1.2 into Go struct field ShotDraft.shots.duration of type int`. The saved run had completed story/scenes and preserved three character profiles. This backend parsing failure predates these frontend changes; no Go files or existing operations were modified.

Severity: high for completing that run. Concrete follow-up: validate/normalize fractional AI shot durations at the structured-output boundary with an explicit rounding/allocation policy that preserves scene timing, then test fractional/invalid values and the isolated fake-native story pipeline. Verify a new isolated run progresses from shots to reference review without submitting video. No paid retry was performed here.

## Unverified

Live model quality, paid generation, real provider recovery, Safari/iOS and screen-reader behavior remain unverified in this round. This is a focused UI refinement, not a new complete application checklist audit.
