# Reference regeneration from nodes — 2026-11-16

User task: regenerate character reference images directly from the canvas, using the existing Library/review workflow.

## Delivered

- Character nodes show a current reference thumbnail, review-needed state and Reference images action. A character-reference image node's Details also offers Character reference images. Both select/focus the same character panel rather than duplicating generation UI.
- The panel shows active images, explicit Generate/Regenerate action with image-charge copy, loading/error/background states and Previous versions. Unsaved profile changes disable generation. Review links open the existing Library preview and approval controls. New images are not automatically approved or used for video.
- Successful manual regeneration retains approved active inputs and adds the new pending candidate, retiring superseded pending/rejected bindings atomically. All asset versions/files remain in Library. Failure leaves bindings untouched. Graph reference edges track active bindings; lookup is linear and layout reserves thumbnail/action height.
- Generation uses only the selected character's saved profile and approved available images. A private, unexported purpose flag filters only the reference-candidate snapshot; JSON clients cannot set it. Ordinary scene/video approval remains enforced. Manual same-character requests are rejected while another is running; automatic missing-sheet generation still waits/reuses.
- Background callbacks recheck component lifetime and character ID after asynchronous refresh. A late result cannot replace another character's gallery, completion notice or typed fields. Leaving the panel stops local tracking without cancelling server work.

## Verified evidence

`2026-11-16-node-reference-regeneration-results.json` records each selected browser check ID, evidence description and result. Expected/reported **94/94 checks**, **8/8 accessibility scans**, **2/2 layout tests**, no remaining selected IDs. Suites: node references 20, canvas editing 26, workspace 48. New cases include both node entry points, paid-action gating, failure retention, draft preservation, refreshed thumbnail/history, delayed refresh after switching characters, Library review, mobile and accessibility. No automatic video requests occurred.

Before repair, Go tests reproduced the pending-reference blocker (`reference pending requires consistency review in Assets`) even for explicit regeneration. Final Go cases pass with approved inputs, no approved input, provider failure, occupied generation lock, cross-project requests, current graph bindings/history, isolation from unrelated saved session selections, and a non-settable JSON purpose flag. The complete Go suite passed in the media-enabled test container; existing automatic character-sheet/approval pipeline cases remain green. Clean Svelte check, production backend/frontend Docker builds, whitespace checks and independent focused review passed.

A browser harness click originally raced the application's legitimate fallback canvas refresh after discarding an edit. The test now checks refreshed thumbnail state and clicks Refresh atomically only if still present. The delayed-reference-response case deliberately pauses the reference refresh, switches to another character and verifies no stale notice/data is applied. No new dependencies or design suppressions.

## Deployment and limits

Deployment followed zero-running-operation/render checks and preserves data/auth volumes. Native post-deploy inspection opened Pip’s reference panel without clicking generation; the assistant titlebar stayed visible, the outer viewport did not scroll, and no console errors appeared. No live provider/native inference or paid retry was submitted. Production native-session review requirements remain in force. Live image quality, charges, Safari/iOS and screen-reader operation are not verified by this fixture-based round.
