# Selectable media providers

User-selected design: work only in JienWeng/videoflow-go; fal.ai defaults for image/video, OpenRouter remains text/vision/transcription, BytePlus ModelArk international is selectable direct Seedance video. No new application dependencies or paid test calls.

The server owns provider credentials, validation, submission and polling. One media-client boundary exposes model capabilities, image generation, video submission and video polling; provider adapters translate the app's canonical payload to the exact provider schema. Text remains on the existing OpenRouter client. The initial catalog explicitly supports known documented endpoints; an arbitrary model string is not assumed compatible.

New default media routes use fal.ai. Existing explicitly saved routes/models remain intact; existing installations with legacy OpenRouter model overrides are migrated to an explicit OpenRouter route rather than sending those model IDs to fal. Each provider retains its own model choices so switching providers cannot pair one provider with another's model ID. Settings shows actual effective routes and per-modality model choices.

Video job provider, model and resolved endpoint are persisted before enqueue/submission. Existing OpenRouter jobs stay OpenRouter. fal request IDs and BytePlus task IDs are durably saved before polling. Settings changes do not redirect existing jobs. Ambiguous submissions require reconciliation, never automatic provider fallback or a second POST. Explicit retries retain the original route unless the user explicitly creates a new request using current settings.

fal uses authenticated queue submission, bounded status/result polling and data URI images. BytePlus uses the ap-southeast ModelArk v3 content generation task API and Bearer authentication. References map to each model's first-frame/reference semantics; unsupported combinations and durations fail before paid submission. Returned provider URLs are not trusted destinations for credentials. Output files are downloaded into existing private local storage and used by the unchanged captions/assembly flow.

Health, provider cards, preflight, character references, storyboard images, guided creation, manual scene/shot renders, retries, reconciliation and model verification must all report/use the selected route. Media catalog validation is not advertised as a successful paid generation or credential check.

Acceptance: distinct counting mock servers prove fal default media, direct BytePlus video selection, retained OpenRouter operation, correct auth and exact payload keys; unsupported route/model/input rejection without paid POST; running jobs poll original provider despite settings changes; restart/lost-response fences remain; browser/settings checks and existing full tests pass. Real account/model access and output fidelity require a later budgeted live check.

Official references (checked 2026-11-11):
- https://fal.ai/docs/api-reference/inference-api/queue/submit
- https://fal.ai/docs/documentation/model-apis/inference/queue
- https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video/api
- https://fal.ai/models/fal-ai/nano-banana-2/edit
- https://docs.byteplus.com/en/docs/modelark/create-video-generation-task-api
- https://docs.byteplus.com/en/docs/modelark/get-video-generation-task-api
- https://docs.byteplus.com/en/docs/ModelArk/seedance-2-0
