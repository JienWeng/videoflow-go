# Component E2E coverage contract

The browser harness is `scripts/e2e_components.mjs`. It uses external Playwright and system Chrome, intercepts API traffic, blocks nonlocal external requests, and never invokes paid inference. Launch the frontend on port 5194, then run:

```sh
PLAYWRIGHT_MODULE=/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs node scripts/e2e_components.mjs
```

`E2E_BASE_URL`, `E2E_OUTPUT`, and `CHROME_PATH` are configurable. Reports and screenshots default to `/tmp/videoflow-component-e2e`. Every inventoried control retains visibility and disabled state; an inventoried control is **not** automatically an exercised control. The JSON `checks` array is the actual executed assertion record. Unknown API endpoints are recorded rather than silently presented as supported behavior.

| Surface | Inventory and deterministic checks | Further integration coverage required |
|---|---|---|
| Shell/navigation | All 12 routes at 1440×1000 and 390×844; command palette opens/closes; primary navigation controls inventoried | Every palette command, onboarding steps, activity operation cancellation/recovery, keyboard focus order |
| Studio | Empty canvas and backend failure inventory | Nodes, edges, drag, zoom, selection, inspector, canvas menus, chat action cards and native director sessions |
| Create | Empty/missing-provider and backend failure inventory; empty Create disabled | Story upload, selected characters, preflight validation, Run/cancel/retry/reload recovery, generated clips |
| Projects | Empty-name disabled; rename cancel/save; delete cancel/confirm; dialog inventory | Cross-host media restoration/full-volume backups; supported same-host JSON export/import is covered in the remaining-risk follow-up |
| Scenes | Empty page and backend failure inventory; generator opens/cancels, empty submit disabled, max-scenes validation | editing, sorting/filtering, shot creation/reorder, confirmation variants, storyboard lightbox, generation jobs |
| Characters | Empty page and backend failure inventory; empty character disabled | CRUD, reference upload/remove, reference lightbox previous/next, generation, visual/voice rule persistence |
| Assets | Empty page and backend failure inventory | Upload types, filter/search, detail metadata, describe dialog, style references, reingest confirmation, delete confirmation |
| Render | Empty page and backend failure inventory | Job progress/error/retry/cancel, scene filters/sort, selected clip order, aspect selection, assembly, original/captioned download |
| Editor | Missing-output response and backend failure inventory | Video playback, scrub, zoom, timeline selection, inspector edits, captions/transcription, original/captioned download |
| Settings | Every tab inventory; Light/Dark/System selection and persistence; backend failure inventory | Provider add/save/remove/test/refresh, masks/labels, model picker/manual input, native engine configuration/run/health, defaults/negatives, agent routing, themes |

The older external `check-personal.mjs` exercises the real Go router with mocked OpenRouter upstream: two-scene generation, selected character/style/aspect/language/duration/direction, render selection retained across galleries, ordered assembly, and editor navigation. It is a separate integration test and must be rerun against the final code before citing a current pass.

T3 preview status was unavailable; preview_open failed with host AppArmor restrictions. The harness uses Playwright with `/usr/bin/google-chrome` as the fallback. Mobile viewport checks establish responsive rendering and control inventories, not touch-device/media-decoder parity.

No claim of every button, popup, state combination, actual model quality, real provider availability, or 100% coverage is made. The integration column is an explicit gap list until supporting execution evidence is added.

Observed initial baseline on 2026-11-11: **30/30 named browser checks passed**, zero page errors, zero unknown fixture endpoints, 68 state inventories and 2,356 control observations (including repeated shell controls). This baseline preceded the new native-engine/director-session integration and must be rerun afterward. `node --check scripts/e2e_components.mjs` also passed. Full evidence is in `/tmp/videoflow-component-e2e/report.json`; per-route screenshots are adjacent.

## Native local sessions

`frontend/src/routes/director/+page.svelte` provides backend-host CLI availability/setup guidance, project session create/select, saved character/asset context, read-only native history, running-turn cancellation and explicit interrupted-turn review acknowledgement. It distinguishes executable detection from account authentication and shows a native conversation ID only after one is returned.

`scripts/e2e_director.mjs` observed **8/8 passing checks**, desktop/mobile, zero page errors. It exercises empty/missing-CLI states, both engines, context restoration/save/warnings, history expansion, cancel/acknowledge, session switching and backend-failure recovery. Evidence: `/tmp/videoflow-director-e2e/report.json` and adjacent screenshots. Frontend diagnostics reported zero errors and warnings after adding this page.

## Real router with deterministic local engines

`internal/api/e2e_fixture_test.go` is skipped unless `VIDEOFLOW_E2E_SERVE=1`. It creates temporary storage/SQLite, a fake Codex JSONL executable, mock HTTP media/transcription/Fal services, a selected Mimi/reference context, and real FFmpeg video/audio fixtures. The production text route executes the fake CLI; it errors if text escapes to the mocked OpenRouter upstream. Fixture-only counters expose actual media submissions; a fixture-only failure switch tests paid-request fencing. No account or real inference is used.

```sh
PATH=/tmp/videoflow-workflow-tools:$PATH VIDEOFLOW_E2E_SERVE=1 GOCACHE=/tmp/videoflow-upload-gocache /home/jienweng/.local/go/bin/go test ./internal/api -run '^TestE2EBrowserFixture$' -count=1 -v
# In another shell, from frontend/:
VITE_API_BASE=http://127.0.0.1:18091 npm run dev -- --host 127.0.0.1 --port 5195
# From repository root:
node scripts/e2e_native_workflow.mjs
# Stop only this fixture:
touch /tmp/videoflow-native-browser-fixture-stop
```

Remove the prior stop sentinel before a new launch. The fixture allows the dedicated frontend origin on port 5195 without changing production CORS. It exits after a bounded wait or its stop sentinel. `ffmpeg`/`ffprobe` must be on PATH; their location above is specific to this test environment. Reports default to `/tmp/videoflow-native-workflow`.

The first integrated run found a real regression: ChatPanel cleared the returned story whenever it equaled the user's message, leaving the Idea field empty and Run disabled. The parent removed that blanket clearing. The rerun preserved the full story, enabled explicit Run, and persisted native turns and two scenes. The integrated route then generated two scene clips using mocked image/video inference, retained render selection across galleries, assembled clips with FFmpeg and opened the editor. A focused real-editor run transcribed the AAC audio, edited the caption, burned subtitles with FFmpeg and downloaded both raw and captioned MP4 files. The pre-review-checkpoint clean run passed all 12 named integration checks with zero page errors (see evidence below).

## Observed integration checkpoint (before reference-review policy update)

- **36/36 component checks passed** after adding Local sessions, Voices and Compositions to the route matrix. Zero browser page errors and zero unknown mocked endpoints; 86 state inventories. This includes 12 routes × desktop/mobile plus explicit validation/dialog/project/theme checks.
- **8/8 Local sessions lifecycle checks passed**, desktop/mobile, zero page errors.
- **12/12 real-router native workflow checks passed at this checkpoint**, zero page errors. Full path: saved local session/context → Studio native proposal with zero media submissions → explicit Run and persisted native thread/history/scenes → two-scene image/video pipeline → render selection and FFmpeg assembly → editor transcription, caption edit and actual FFmpeg burn → raw/captioned MP4 downloads. Same generation request key did not resubmit images/videos. Saved-voice preview did not submit; explicit Run produced a locally imported audio asset; duplicate completed Run did not submit again. A simulated provider HTTP 500 submission was counted once; a repeated Run remained fenced and did not resubmit. Composition planning stayed proposal-only, then Save created a reopenable source revision; rendering stayed disabled with the optional runtime missing. Populated routes were inventoried at desktop/mobile widths (22 additional route/viewport observations).
- All three new browser scripts passed `node --check`. No application package dependencies were added.

Exact reports: `/tmp/videoflow-component-e2e/report.json`, `/tmp/videoflow-director-e2e/report.json`, `/tmp/videoflow-native-workflow/report.json`. The native report records HTTP mutations and inventories; adjacent files include screenshots and the actual downloaded raw/captioned videos. `component-e2e-controls.md` records observed visible controls by route. Counts of control observations include repeated shell controls and are not unique component coverage percentages.

The final integration used a missing optional HyperFrames runtime to verify its guard; it did not run HyperFrames rendering. The composition integration agent separately owns actual local-runtime evidence. Voice clone, native reference generation, lip sync, alignment/listening approval and export variants have backend test evidence from the voice integration agent; the browser workflow here covers saved-voice speech. Provider billing, native subscription sign-in, actual model quality, complete browser/device combinations, every canvas gesture, and every conditional popup remain outside this deterministic browser run. The control inventory and integration gaps above must remain visible when reporting coverage.

## Focused identity and character-upload checks

`scripts/e2e_identity_upload.mjs` exercises real API-backed identity locks and the original reference-photo upload path. The first focused run verified identity PUT revision advancement, reopened persisted values, discarded unsaved trait removal, delete-dialog cancellation, real file-chooser upload, reference count persistence, image decoding and reload. A separate native-context check verified that explicit Photo selection was saved and the next native turn recorded its `locked_traits.color` as `crimson`.

The invalid-image case exposed a real defect: arbitrary text uploaded with a PNG filename was accepted and attached as a character reference. The parent added bounded image content/header/dimension validation; the focused rerun now passes with HTTP 400 and no reference attachment (RED→GREEN). Earlier thumbnail failures were fixture-only: the storage path was set after static routes captured it; the dedicated fixture helper now sets a production-style `<temp>/storage` root before server construction. The uploaded valid PNG is produced by Chrome, and fixture reference/keyframe PNGs are produced by FFmpeg.

The parent subsequently added a required generated-reference approval gate. The earlier 12-check pipeline checkpoint predates that policy and is archived at `/tmp/videoflow-native-workflow/pre-reference-review-report.json`; it must not be presented as current approval-gate verification. The current focused approval-gate evidence below supersedes its automatic-render behavior. No broad route inventory rerun is needed for that behavior change.

## Current focused regressions after final policy changes

- `scripts/e2e_identity_upload.mjs`: **4/4 passed**, zero page errors. Real asset identity PUT advances the revision and persists reopened values; unsaved trait removal is discarded; asset delete Cancel preserves the asset. A real file chooser uploads a valid PNG, increments the saved character reference count, displays a decodable thumbnail and survives reload. A text file named PNG returns 400 without attachment. Explicit local-session Photo selection is saved and the next recorded native chat request contains its crimson identity lock. Evidence: `/tmp/videoflow-identity-upload-e2e/report.json`.
- `scripts/e2e_reference_review.mjs`: **4/4 passed**, zero page errors, resuming the existing generated-keyframe checkpoint to avoid repeating image generation. The original Create call had already produced two images and zero videos. The resumed run verifies that pending review survives reload, pending/rejected render requests fail, the normal Assets Reject/Approve controls are reachable and generated images decode, approval itself submits no video, and explicit Render creates exactly two successful jobs. Both durable job payloads specify `generate_audio:false` and `voice_policy.mode:saved_voice_visual_pass`. Evidence: `/tmp/videoflow-reference-review-e2e/report.json` (`resumed_from_existing_keyframes:true`).

That regression exposed another real defect: the asset detail dialog grew beyond the viewport when generated metadata contained the full context snapshot, leaving review buttons unreachable. The parent bounded the dialog height and enabled scrolling. The same normal-click review interaction then passed (RED→GREEN); the test uses no force-click workaround.

The full native harness source is updated for the approval gate. Its earlier end-to-end report is a prior checkpoint, while the focused report verifies the changed behavior. No broad route matrix was repeated after the bounded policy fixes. The fixture used for the focused run was built at 19:43 local time; a later unbound-speaker voice-policy refinement was covered by the parent's Go checks rather than this saved-character browser path.

To run only review interactions against an existing two-keyframe unapproved fixture checkpoint:

```sh
E2E_RESUME_REVIEW=1 node scripts/e2e_reference_review.mjs
```

The normal command without that variable performs Create first. All five browser scripts (`e2e_components`, `e2e_director`, `e2e_native_workflow`, `e2e_identity_upload`, `e2e_reference_review`) require the external Playwright module; no app dependency was added.


## Remaining-risk follow-up (2026-11-14)

[Targeted follow-up evidence](2026-11-14-remaining-risks.md) covers same-host JSON download/import, media binding validation, uncached backend recovery/voice tests, real optional HyperFrames rendering, Chromium scale checks, Linux WebKit and automated accessibility scans. It supersedes the corresponding historical gap entries above; real account-backed native AI, Safari/iOS hardware, screen readers and provider quality remain outside the run. The follow-up also records the ARIA, focus and contrast fixes discovered by populated/light-theme testing.
