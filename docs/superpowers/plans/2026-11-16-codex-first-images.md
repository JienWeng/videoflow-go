# Codex-first image routing

Use the installed Codex CLI's built-in image_gen tool first for local Codex sessions, including character sheets, props, locations and storyboard images. Keep native text planning restricted. Add an image-only invocation with shell/search/apps disabled, staged reference inputs, and explicit JSON output. Import only a fresh validated raster file from Codex's generated_images directory. Never accept an arbitrary assistant-reported filesystem path.

Fallback to the saved image API only for a server-detected unavailable image tool or unsupported input before the native turn. Assistant-reported unsupported after a native turn is ambiguous and must stop without fallback. Authentication, timeouts, native failure, incomplete output and ambiguous generated results stop the request instead of creating a second paid image. Preserve current asset metadata, approval gates and reference identity snapshots. Display the route and fallback policy in Create story; record the actual image provider and fallback reason per saved asset.

Verify native success makes zero image-provider requests, unsupported capability uses existing API, failure never falls back, paths/files are constrained, current references are passed, and existing pipeline tests remain valid. Do not reset the user's native session-review gate. Update PR #2; no merge.
