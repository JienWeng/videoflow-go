# Local composition boundary

Docker bundles the pinned HyperFrames runtime, Chromium and FFmpeg/FFprobe.
For native/source deployments, the optional runtime needs Node 22+ and local Chrome. Run
`scripts/setup-hyperframes.sh /absolute/app-owned/runtime` explicitly, then set
`VIDEOFLOW_HYPERFRAMES_BIN=/absolute/app-owned/runtime/node_modules/.bin/hyperframes`
and `HYPERFRAMES_BROWSER_PATH` to the existing Chrome executable. No runtime
installation occurs while handling requests. Version 0.9.33 is required.

`composition.InstallSkills(appOwnedProjectWorkspace)` installs the vendored
Apache-2.0 upstream skills in both project skill locations. Upstream source is
pinned at e596be16c901639ce6c18032e56924a9d8e1967d; project overrides supersede
upstream paid services, telemetry, transcription, downloads and publishing.

`GET /compositions/sources` lists existing project assets and completed render outputs.
Output source IDs use `output-<output-id>` and are resolved only through their
project-owned render job; public `storage/...` paths use the shared storage resolver.
Client-provided file paths are never trusted.

Routes: `GET /compositions/runtime`, `POST /compositions` (version-1 manifest),
`GET /compositions/{revision}`, `POST /compositions/{revision}/run` with
`{"operation":"lint|check|snapshot|render","revision":"exact saved revision"}`.
The active project is resolved once and captured before work. A new manifest
revision is required after edits. Imported sources are copied and hashed;
modified revision files are rejected before local operations. Render imports
create separate derived assets, with the manifest revision/source references.

The `/compositions` page provides clip/audio timing rows, captions and format
presets. A local-session plan fills a reviewable form without saving or rendering;
raw manifest editing stays under Advanced. Approved aligned captions can be
reused from an owned output through `POST /compositions/captions/import` with a
clip. Save and Run validate exact cue wording/timings, speaker/language, upstream
script/audio bindings, output/caption hashes and clip timing. Partial-cue trims
require source review rather than silently truncating wording. Version 1
supports normal-speed video/image layers, separate audio, and explicitly timed
caption cues. Unsupported speed changes fail preflight. Original media duration
and dimensions belong in source metadata. No OpenCut adapter, lossless HTML
round trip, arbitrary HTML editor, transcription or paid alignment is implied.
Caption wording/timing approval remains the script/audio workflow's authority.

Tests: `go test ./internal/composition`; opt-in local lint/snapshot/render uses
`VIDEOFLOW_HF_TEST_MEDIA=/path/to/two-second.mp4` and the configured runtime.


The browser composition editor previews existing media, trims source ranges, splits and duplicates clips, reorders layers, and records undo/redo history. Fit/fill, zoom, focal position, brightness, contrast, saturation, opacity and mute are persisted on each clip. Framing/color are non-destructive composition edits, not mutations of identity reference files. Source metadata is probed locally through a project-scoped endpoint. Saved HTML includes only validated fixed CSS values, never arbitrary authored HTML or JavaScript. Legacy manifests without adjustments retain their original document bytes and revision hashes.

Node editing routes share the existing Library, scene studio, caption editor and composition editor. A scene opens its latest completed render by default; other versions can be added from the source chooser. A render job exposes status and related tools rather than editing immutable output internals. Playback speed remains 1; transitions and arbitrary custom HTML effects are not supported by this adapter.
