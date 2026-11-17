# Duplicate scene video submissions

Read-only production inspection found three successful whole-scene jobs submitted within 16 seconds, each requesting eight seconds with no shot ID. The latest two frozen provider request bodies were identical. These are separate generated scene versions, not three short shot clips for assembly. Provider billing totals were not inspected. No live generation was submitted during investigation.

Both scene-render entry points now share a scene lock and check existing jobs before queueing another video. Identical pending, running, reconciliation-required or completed jobs are reused; a different active/unresolved request blocks another submission. A completed identical render can be replaced only with the explicit new_version option, exposed as a paid-version checkbox in chat. Failed jobs remain retryable. Existing outputs are preserved.

The chat explains that scene rendering produces one whole-scene video from the shot plan. Per-shot rendering and clip assembly remain separate existing workflows; no automatic batch of paid shot generations was introduced.

Verification: focused duplicate-state tests cover queued, running, completed, uncertain and failed jobs, changed inputs, explicit new versions and JSON key-order independence. Existing storyboard-preparation integration verifies repeated actions retain a single video job and disclose reuse. Provider calls use fixtures only.

Results: full Go suite passed; focused final API tests passed; Svelte check found zero errors and zero warnings. This is duplicate suppression within the single backend instance, not distributed multi-server coordination. Explicit alternate-version requests preserve the ability to regenerate after a completed render.
