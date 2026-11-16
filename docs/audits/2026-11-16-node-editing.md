# Unified node and manual video editing — 2026-11-16

Main task: move from a story/canvas item to the same full manual editing tools used elsewhere, and edit/render video without requiring an AI turn.

## Implemented capability map

| Item | Tools available from the node/workflow |
| --- | --- |
| Character | Full character Library; appearance/personality/visual and voice rules; reference generation/version review; voice and dialogue workspace |
| Scene | Full scene studio; cast, shots, props and location references; storyboard review; render; open latest completed clip in compositions |
| Shot | Prompt, integer duration, camera, motion and revision history; parent scene studio deep link opens/focuses the selected shot |
| Asset | Name, description and identity locks directly in node details; full Library for usage, review and versions; image/video/audio composition entry where supported |
| Output | Caption/transcription/QA editor and direct trim/arrange composition entry |
| Render job | Status, related scene and renders/outputs; generated job internals are not editable media |
| Composition | Live layered preview; seekable visual timeline; trim/start/length; split, duplicate, reorder, remove; undo/redo; image/video fit/fill, zoom, focal position, brightness, contrast, saturation, opacity, source mute; audio placement/trim; caption source links; immutable saved revisions and local render |

Shared routes and source IDs reuse existing records instead of creating another Library or editor. Explicit composition source deep links probe only project-owned media, use its actual duration, and preserve the original file. Scene shortcuts load the latest render; other versions remain available in the source chooser. Source URLs are resolved by the backend storage boundary, never client file paths.

Framing/color changes affect the composition, not canonical character/asset reference images. Review gates remain in place. Approved caption bindings must be removed and reimported when splitting or changing linked timing. AI planning sees the same complete adjustment schema and returns a proposal; it does not automatically save or render.

Docker now bundles HyperFrames 0.9.33, Chromium, FFmpeg and FFprobe. Native/source setups retain the documented explicit runtime setup route. No dependency installation occurs during an API request.

## Verified evidence

- Full Go suite passes. Targeted API/composition checks repeated after adding planner guidance.
- Svelte check: zero errors/warnings; production builds pass.
- Five pure tests cover all six node route types, source ID encoding, split offsets, independent duplication and preview adjustment format.
- `scripts/e2e_node_editing.mjs`: **24/24** checks at 1440px and 390px, **2** WCAG A/AA axe scans with **0** violations, **0** page errors. Exact IDs and per-check results are in `2026-11-16-node-editing-e2e.json`.
- Existing composition browser workflow passes on both desktop/mobile, including reviewed AI proposal, manual editing, save/run gating and linked captions (mocked planner/runtime).
- Real local pinned HyperFrames integration: structure lint, snapshots and MP4 render pass; adjusted render pixel check verifies brightness=0 reaches exported pixels. Log: `2026-11-16-node-editing-render.txt`. The adjusted-render pixel test also passes inside the packaged Docker image as the application user, using its bundled Chromium.
- Tests confirm manual edits do not change fake AI/provider counters. Production data and native review gates were not modified by tests.
- Independent read-only review found early undo capture; microtask history batching fixed it. Proportional caption sizing, metadata-triggered trim sync and duplicate action removal were reviewed; no remaining important findings.

## Practical limits / unverified work

This provides the documented adapter capabilities, not every upstream HyperFrames/desktop NLE feature. Playback speed remains 1. Transitions, arbitrary HTML/JavaScript effects, video retouching and source-image pixel export are not implemented. Live browser preview is for editing; saved rendered frames/exports are the final verification surface. Live paid generation and model adherence to requested edits were not exercised. This is not a claim of zero bugs across every possible file/device.
