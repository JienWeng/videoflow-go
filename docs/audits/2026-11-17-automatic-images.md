# Automatic images, without mandatory review

User requested removal of the image review flow. New story runs now generate characters, props/locations and storyboard images together, then finish ready to render. Images are usable as soon as generation succeeds. Legacy pending images are usable; saved stories paused at the old character checkpoint can continue without approving images. Existing operations are never restarted automatically on page load.

Removed mandatory approval controls and progress stages from chat, Create story, character images and Library, plus pending-reference approval alerts on the dashboard. The redundant proposed-workflow review trace was removed from chat. Scene studios and canvas labels indicate availability rather than demanding review. Viewing, regeneration and version history remain. Optional exclusion/allow-reuse lives in Details or Image options; explicitly excluded images remain blocked, including after editing descriptions/traits. Regeneration replaces generated active candidates while preserving imported/pinned references and image history.

Video submission remains a separate explicit action. Project ownership, missing files, stale storyboard revisions, locked identity, native account/interrupted-session recovery, voice consent and uncertain provider-submission handling remain authoritative.

## Verification

- Added failing-then-passing regression for ready and legacy pending images being usable without approval; rejected images remain unusable.
- Full Go suite passed. API suite passed again after the final exclusion-on-edit regression.
- Existing pipeline tests now prove automatic references and storyboards, preserved work after failure, fractional shot durations, prop reuse, no automatic video calls, and legacy continuation with a pending replacement and duplicate-continuation protection.
- Svelte: 0 errors/warnings. Production builds passed.
- Real-router desktop/mobile: 14 checks and 6 axe scans, no browser errors/violations. Verifies project style, explicit start, automatic complete image pipeline, storyboard readiness, canvas focus, full images without approval controls, regeneration and standalone images.
- Asset viewer desktop/mobile: 6 checks and 4 axe scans, no browser errors/violations. Verifies simple/detail modes, preserved drafts, collapsed metadata, optional exclusion/reuse and reset-on-reopen.
- Final production frontend chat-state regression: 20 checks, no browser errors; all API calls intercepted.
- Deployed backend and frontend after verifying no active operations/render jobs. Local and Tailscale health passed; readonly production asset viewer showed ready-to-use images, Simple/Details and no approval controls, with no console errors.
- Pure dashboard/guidance/progress tests passed (18 test entries, including progress assertions).

Tests used isolated data and fake providers; no live paid generation or model-quality assessment. Optional image exclusion retains the existing review API for compatibility; old status keys remain readable. Native session recovery and voice approval are distinct from the removed image-approval flow.
