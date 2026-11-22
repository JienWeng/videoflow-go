# Interface QA — 12 November 2026

Reviewed the browser app from source revision `e93ba20`, then rebuilt the Docker instance with the fixes below. Used the T3 collaborative browser at 1280 × 800 and an emulated 390 × 844 viewport. This was a functional and usability pass, not a claim that every provider or every populated editor state was exercised.

## Verified defects fixed

- **Project creation:** the screen promised to switch to the new workspace, but only created it. The frontend now activates the returned project before navigating. Verified against the live active-project endpoint after clicking Create project.
- **Project rename:** PATCH treated an omitted description as an empty description. Omitted fields now retain their existing values. A failing regression test reproduced the loss; both the corrected test and a live rename preserved the project notes.
- **Project import:** the file picker accepted ZIP and sent multipart data to a JSON endpoint. It now accepts JSON, parses the file, and sends the supported format, with an actionable parse error.
- **Import data loss:** importing an existing export reused primary keys and ignored insertion errors, reporting success while losing characters and scenes. Imports now allocate fresh IDs, remap references, and write the entire snapshot in one transaction. Invalid imports do not leave empty or partial workspaces. Exports include scripts, shots, assets and style as well as scenes and characters. Tests cover same-database round trips, character/asset binding, cast, shots, style, invalid files and transaction rollback. A real browser download/upload round trip retained a character, image and its locked color trait.
- **Export behavior and scope:** Export now downloads a named JSON file and surfaces request failures. The interface explicitly says that media files, render history, voices and AI sessions are not bundled. Referenced media must exist inside the backend storage root; JSON export is not a portable media archive or a full backup.
- **Project counts:** removed fabricated zero counts when the API does not supply counts.
- **Mobile Settings:** provider and model controls in agent routing overflowed their container. Controls now wrap and shrink. All four selectors were present and inside the 390 px viewport on the rebuilt app.
- **Mobile tools:** Search and Activity were only available in the hidden desktop sidebar. Both are now in the mobile header. The Activity panel opens within the viewport; the search palette opens and closes with Escape.
- **Onboarding:** replaced promises of automatic rendering with the actual setup, reference approval and separate render steps. Removed the empty third grid column. Used the shared Dialog component for focus management and bounded scrolling. Verified focus inside the dialog, viewport bounds, Escape dismissal and persisted dismissal.
- **Guided creator:** removed the repeated effective-style paragraph and moved model IDs and clip limits into a disclosure. Unknown clip durations have useful fallback text. Links to local session setup now say “Open AI sessions.”
- **Composition sources:** images appeared in the audio-source dropdown. The backend now supplies a media kind derived from the stored file, and the frontend separates visual and audio sources. Verified that the uploaded PNG appeared only in the visual dropdown.
- **Deployment caching:** cached HTML could load removed JavaScript chunks after a rebuild, leaving the page blank. HTML now uses `Cache-Control: no-cache`; versioned assets use immutable caching and missing asset paths return 404 instead of the SPA HTML fallback. Verified response headers, missing-file response and Nginx configuration.

## Browser coverage

Visited Studio, Projects, Characters, Assets, Scenes & shots, Videos, Guided creator, Voices & captions, Compositions, AI sessions, Settings and an unavailable editor-output route. Swept the main layouts at desktop and mobile widths.

Exercised project creation/switching/rename and JSON export/import; character creation and appearance editing; image upload and thumbnail/detail preview; asset identity-lock editing; spoken-line draft/save; composition layer selection, save and saved-revision reload; Settings sections and Light/Dark appearance; keyboard command search; scene creation dialog open/cancel; missing-output recovery; mobile Studio tabs; mobile Search and Activity; onboarding focus and dismissal.

Temporary QA workspaces were deleted after testing and Default Project was restored. No paid model generation was submitted.

## Verification

- `go test ./...`: passed in the backend runtime with a Go 1.26 toolchain, FFmpeg/FFprobe and Python available. Initial runs in incomplete test images failed because FFprobe or Python was missing; the final run included both.
- `npm run check`: zero errors and zero warnings.
- Docker Compose rebuild: passed; backend and frontend are running.
- `nginx -t`: passed.
- Impeccable detector over changed frontend targets: no deterministic findings.
- `git diff --check`: passed.
- Tailscale HTTPS UI and `/api/health`: responded successfully after deployment.
- Independent read-only code review: no critical or important introduced defects found.

## Remaining limits

Live image/video generation, transcription, voice cloning, speech and lip sync require provider credentials and were not tested. No real model quality or account access is claimed. The Docker backend has no configured local Codex/Claude CLI, so planning and native session inference remain unavailable on this instance. The optional composition renderer also needs setup; saving an edit was verified, actual composition rendering was not. Finished-output editing, populated scene progression and real touch gestures need a later pass with suitable fixtures/runtime access. JSON exports remain same-host story snapshots with media references, not complete portable backups.
