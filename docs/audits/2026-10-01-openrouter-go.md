# OpenRouter-only Go implementation — 1 October 2026

The Go backend now routes AI tasks through OpenRouter. This report supersedes the 27 September compatibility report, whose implementation claims did not match the initial Go migration.

## Changes

- Database credentials and agent/default models resolve at call time. Named OpenRouter connections have independent keys and models. Arbitrary and legacy connection URLs are checked before a credential can be sent.
- Connection checks call `/key`; discovery fetches the current task-specific model catalogs. Text/JSON verification performs a real small completion. Media verification checks catalog membership and does not claim to generate media.
- Agent/provider failures propagate; deterministic scripts, generic assistant replies, sample images/videos, fixed QA scores, and sample transcription responses have been removed.
- Character sheets, planned props and storyboards generate actual image assets. The prompt agent prepares image prompts. Character references are linked to characters; storyboards use the asset type expected by the frontend.
- Video jobs validate model-specific parameters, submit, persist their upstream ID and actual request, poll, and download authenticated content. Non-video responses are rejected. Jobs resume after shutdown; explicit retry creates a fresh generation. Partial multi-output downloads are continued without treating the first saved output as completion of the whole job.
- Storyboards can supply a first-frame anchor when advertised by the selected video model. Explicit local image references are converted to data URLs; unsupported audio/video references fail.
- Frame QA and asset/style recognition use image inputs on OpenRouter Chat Completions. Transcription uses OpenRouter's dedicated endpoint with actual segment timestamps. FFmpeg performs extraction and caption burning, including supported caption styles.
- Settings expose OpenRouter and its live image/video/vision/transcription catalogs. Saved scene duration, dialogue language, negative prompts and caption styles have runtime consumers. The retired AtlasCloud adapter was removed.

## Verification

Provider, routing, API and worker contract tests cover invalid credentials, unsupported models/parameters, JSON retries, image outputs, genuine transcription timestamps, persisted credential/model selections, legacy URL rejection, provider failure propagation, restart without duplicate submission, cancellation, retry semantics, and storyboard forwarding. A mocked guided workflow runs through script, shots, image generation, video submission and authenticated download.

Real FFmpeg tests generate an audiovisual fixture, extract WAV audio, burn captions, and verify different supported styles produce different output files. Tests use a temporary distro FFmpeg and libraries because FFmpeg is not installed on the host's normal PATH.

Final checks: `go test -race ./...`, `go vet ./...`, backend build, Svelte check, and production frontend build. All passed. The concurrent background-task regression also passed 30 consecutive race-enabled runs after constraining SQLite to its initialized connection.

A temporary isolated Go server was started without an environment key; the locally saved OpenRouter key was supplied to its settings API. The updated adapter successfully authenticated against OpenRouter. Live catalog results: 55 image models, 30 video models, 295 vision models, and 24 transcription models. Live preflight passed for the default image/video/text models and 9:16 aspect ratio. The original database and stored assets were not changed by this smoke check.

## External limits

No paid live generation was submitted. Account credit sufficiency, live generated quality, real reference-video acceptance, and provider timestamp behavior remain live account/model checks; catalog membership and mocked contracts do not certify those outcomes.

Frame QA assesses three still frames, not continuous motion or audio. FFmpeg with libass must be installed on the deployment machine for frame/audio extraction and caption burning. Transcription requires a model with `verbose_json` segment support and currently accepts extracted audio up to 25 MB. Karaoke highlighting is not advertised; supported styles are clean, bold, minimal, cinematic, neon, kids, classic and comic.

Guidance references require capability metadata that advertises their support. OpenRouter's current video catalog advertises frame anchors; its guide demonstrates remote URLs, so local data-URL video references remain model-dependent. There is no automatic paid retry after an ambiguous submission failure. Restart recovery can reuse an upstream ID once it has been persisted; a crash between remote acceptance and local persistence cannot guarantee exactly-once submission.

Official contracts: [images](https://openrouter.ai/docs/guides/overview/multimodal/image-generation), [video](https://openrouter.ai/docs/guides/overview/multimodal/video-generation), [speech-to-text](https://openrouter.ai/docs/guides/overview/multimodal/stt).

## Local beginner downloads

The macOS (Apple Silicon and Intel) and Linux (AMD64 and ARM64) archives contain the Go app, compiled browser interface, a launcher and beginner instructions. No Go or Node installation is needed. Default desktop binding is loopback; Host and Origin checks reject unrelated sites before they can dispatch paid generation. Downloaded packages keep projects and credentials in the user configuration directory, separate from the extracted folder.

The extracted Linux AMD64 archive was launched from a different working directory with an isolated user configuration folder. Its SPA and API responded, a persistent database was created, and no saved key was packaged. macOS and ARM64 binaries were cross-compiled, but not run on those systems. No browser automation surface was available, so interface verification used production build checks and HTTP route checks rather than a rendered browser session.

The app is running locally at http://127.0.0.1:8000 using the existing workspace database and storage. No server deployment was performed. FFmpeg is available to this local process through the temporary distro tools used for verification. Downloaded packages do not bundle FFmpeg or code signing.
