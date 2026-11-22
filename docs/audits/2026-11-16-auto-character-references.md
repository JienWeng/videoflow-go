# Automatic character reference sheets — 2026-11-16

Requested behavior: generate character reference images as part of local Codex story creation, instead of requiring a separate manual generation task. Native Codex/Claude plans the prompt; the configured reference-image provider draws it.

## Implemented workflow

- Local guided runs create any needed character profiles and shots, then generate sheets for cast members with no reference bindings. Existing bindings/identities are reused. Each generated image is saved in Library and appended to its correct character. Per-character locking prevents simultaneous automatic runs from independently generating the same missing sheet.
- Generated candidates remain pending. The operation saves its cast, sheet IDs, scenes, brief, media routes and stages, then pauses at awaiting_character_review. The UI shows eight steps, review links/thumbnails and a gated Continue storyboarding action; image-call costs are explained before submission.
- Continuation checks project ownership, current scene cast, approved/available current references and frozen media routes. An atomic exact-snapshot claim gives only one request ownership before paid work. The same operation resumes only scene image generation, preserving profiles, sheets, script and shots. Duplicate clicks cannot repeat it. Scene review and explicit video rendering remain later gates.
- Reference replacement is supported: refresh reloads current character bindings as well as assets. Storyboard briefs are rebuilt from current cast data, and local image inputs come exclusively from the current validated director snapshot. Detached references from an old saved brief cannot silently condition new images.
- Failed approval refresh does not submit using stale state. Missing bindings link back to Characters for repair. Partial failures preserve earlier sheets and story records. No automatic retry, voice setup or video submission was added.
- Preflight validates the reference route's square format and the image route's conditioning support before planning/inference. Older saved runs retain their original progress view; legacy nonlocal creation copy remains accurate.

## Evidence and counts

`2026-11-16-auto-character-references-results.json` records every browser audit ID, description and result. Expected/reported **96/96 checks**, **12/12 accessibility scans**, **18 state assertions**, no remaining selected IDs. Browser suites: new reference flow 20, existing progress 20, workspace regression 48, isolated real-router dog workflow 8. New flow covers Library approval, restored operation state, current replacement bindings, refresh/continuation errors, no duplicate generation, both review gates and mobile action visibility.

Full Go suite passed in the media-enabled test-tools container. Focused preflight regression also passed: unsupported square format leaves zero characters/scenes/operations and makes no inference call. Fake-native integration cases cover two character sheets, blocking unapproved/cross-project/duplicate continuation, current replacement inputs, sheet reuse, exact shot totals, first-sheet retention on a later sheet failure, and late storyboard failure retention. Eight concurrent DB claim attempts produce one continuation owner.

The documented isolated browser fixture ran separately with real SQLite, Go routes and FFmpeg, fake native Codex and fake provider HTTP. Both widths passed: profile opt-out produces one storyboard and no cast; enabling profiles produces two dog sheets, pauses for approval, then produces one conditioned storyboard. Total image count increases by four across those two requests; video count stays unchanged. The fixture and its Vite server were stopped afterward. No live project or paid provider was used by testing.

Production backend/frontend Docker builds, clean Svelte diagnostics and whitespace checks passed. Independent review closed two real issues (stale UI approval IDs and stale scene reference inputs) and reported no remaining important findings. A workspace run under concurrent build load completed its cases but reported two transient module-fetch errors; unchanged isolated rerun passed all cases with no page errors. No new dependencies or design suppressions.

## Deployment and limits

Verified zero live running operations/renders before replacing backend/frontend. Existing data/auth volumes are preserved; health checks pass. Production preflight recognizes automatic reference generation. The user's selected native session is currently review_required, so starting a new run is correctly blocked until the interrupted request is reviewed in AI sessions; this gate was not cleared automatically. Existing failed receipts are not rewritten or retried.

Live model quality, actual provider charges/availability, Safari/iOS and screen-reader operation remain unverified. The request adds automatic generation to new local guided runs; it does not retroactively restart a failed operation. Existing saved cast may be selected in a new run to reuse profiles and available references.
