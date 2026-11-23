# Provider routing

This guide describes the provider architecture on current source. The v0.3.2 browser preview predates the fal/BytePlus integration; rebuild current main or use a later release that includes it.

| Task | Fresh-install default | Other supported provider |
| --- | --- | --- |
| Story, chat, shot/prompt writing | Selected local Claude Code/Codex session | Select a different native session |
| Vision/QA | OpenRouter | Named OpenRouter connections |
| Transcription | OpenRouter | — |
| Characters, props, locations, storyboards | Codex built-in image generation when a capable local Codex session is selected | Configured fal.ai/OpenRouter image route for unavailable or unsupported native input |
| Scene/shot video | fal.ai | OpenRouter or BytePlus ModelArk international |

## Set up and switch

Open Settings → Providers. Set up a local text engine in [Local sessions](LOCAL-DIRECTOR.md). Save an OpenRouter key for vision/transcription and a fal.ai key for images/video. Save a BytePlus ModelArk key if you want direct Seedance video. Keys are sent only by the local backend. Environment alternatives are `OPENROUTER_API_KEY`, `FAL_KEY`, and `BYTEPLUS_API_KEY`; their presence is not proof of model access.

In Settings → Engines, select Image provider and Video provider independently, then choose a model from that provider's supported catalog. Switching providers restores its previous model choices. Image/reference model IDs cannot be sent to a video-only provider, and a model ID from one provider is rejected under another. All text planning/writing uses the selected native session; legacy cloud-text route records do not override it.

Media **Check configuration** verifies the locally saved configuration, not the account's permissions or a paid generation. Media **Verify** checks the supported model profile without submitting generation. OpenRouter's text-model verification is a small billable completion. A later small, budgeted generation is needed to verify real account/model access and output quality.

## Codex-first images

A capable selected Codex session uses the CLI's built-in image_gen tool first. Character sheets, props, locations and storyboards share this route. It uses the signed-in native account, not an image API key passed by VideoFlow. The planning turn keeps image generation disabled; a separate restricted image turn receives the current reference images and returns a file for validation/import. Generated assets still require reference approval.

The image API is a fallback only when the backend detects missing native capability or unsupported input before starting the image turn, such as more than five reference images. Once a native turn starts, errors and assistant-reported unsupported results pause for review and never automatically trigger another paid image request. Claude Code sessions use the configured image API. Video still uses the selected video provider.

Create story shows this route and fallback policy. Saved asset metadata identifies the provider actually used. CLI capability detection is not proof of account-level image access: a real native generation is needed to verify that. A session marked review_required must be reviewed by the user before any new native work.

## Initial supported profiles

| Provider | Default model | Behavior |
| --- | --- | --- |
| fal.ai images | `fal-ai/nano-banana-2` | Text-only requests use this endpoint. Reference-conditioned requests use its documented `/edit` sibling with `image_urls`; at most 14 references. Explicit `/edit` selection requires references. |
| fal.ai video | `bytedance/seedance-2.0/fast/image-to-video` | Uses the approved scene keyframe as `image_url`; optional last frame maps to `end_image_url`. 4–15 seconds, 480p/720p. |
| fal.ai video alternative | `bytedance/seedance-2.0/fast/text-to-video` | For renders without a keyframe; reference frames are not silently discarded. |
| BytePlus video | `dreamina-seedance-2-0-260128` | Direct international ModelArk task API, with typed text/image content and explicit frame roles. This profile exposes 4–15 seconds and 480p/720p. |
| OpenRouter | Existing configured models | Discovered/validated through the existing modality-specific catalogs. |

These are explicit supported profiles, not a promise that every arbitrary fal or BytePlus model ID accepts the same input schema. Adding another model requires its documented payload/capabilities and adapter tests. Account availability remains provider-controlled. No prices or cheapest-model claim are inferred from these defaults.

The official endpoints are `https://queue.fal.run`, `https://ark.ap-southeast.bytepluses.com/api/v3`, and `https://openrouter.ai/api/v1`. BytePlus here is the international service, not mainland Volcengine Ark. Do not reuse Volcengine model IDs/endpoints under this connection.

## Saved work and paid-job safety

Fresh unselected media routes default to fal. Existing explicit OpenRouter routes remain OpenRouter. Legacy saved image/video model overrides are given an explicit OpenRouter route during migration. Existing jobs retain their original provider, model, endpoint and upstream ID.

Guided creation captures its media routes before beginning paid stages. Later Settings changes affect new requests, not that pipeline or a submitted video job. Provider request IDs are stored before polling. Re-render is an explicit new paid request on the original route. Unknown video submission results stay fenced for reconciliation; video jobs never automatically submit to another provider as a fallback.

My videos shows the saved job/provider and can resume a verified existing provider ID. This resumes polling only. fal's queue status `COMPLETED` is followed by result retrieval; the result can still report failure. BytePlus task states map to the app's pending/running/completed/failed states. Saved outputs continue through the existing editor, caption burn, assembly and download paths.

Provider keys are not attached to output downloads, and production output URLs cannot reach local/private services. Selected local references are encoded for the generation request; localhost storage URLs are not sent as if they were publicly reachable.

## Verification

Local tests use separate counting provider mocks, real FFmpeg and browser checks. They verify auth/payload translation, defaults and migration, provider/model switching, reference conditioning, pipeline/worker route snapshots, failure/recovery behavior and downloadable outputs. No paid generation is used by these tests.

Official references: [fal queue API](https://fal.ai/docs/documentation/model-apis/inference/queue), [fal Seedance image-to-video schema](https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video/api), [fal image-edit schema](https://fal.ai/models/fal-ai/nano-banana-2/edit/api), [BytePlus creation API](https://docs.byteplus.com/en/docs/modelark/create-video-generation-task-api), [BytePlus retrieval API](https://docs.byteplus.com/en/docs/modelark/get-video-generation-task-api).

Known advanced-API limitation: omit `seed` for the current BytePlus Seedance 2.0 profile. The adapter currently forwards an explicitly supplied seed even though that model does not document it; the guided UI never supplies seed. fal's overview and detailed schema differ on seed support, so no live seed guarantee is made.

## Current connected video/audio flow

See [Connected video workflow](CONNECTED-VIDEO-WORKFLOW.md). On supported models, scene render plans default to native audio, with optional music direction; saved-voice and silent modes remain explicit. Saved voices are only prerequisites for saved-voice mode. The worker preserves the selected mode and the local unit/plan metadata, and strips local metadata before contacting providers. Automatic grouping minimizes calls and generated seconds, and matching planned clips are reused. The composition editor preserves clip audio and applies uploaded/library music gain/fades to both preview and local export. Existing silent videos do not acquire native sound by changing a setting.

### Direct OpenAI image fallback

Save a key under **Settings → AI connections → OpenAI Images**, or provide
`OPENAI_API_KEY` to the backend. The configured OpenAI route takes precedence
over the regular image provider when native Codex images are unavailable or the
inputs are known to be incompatible. It handles both prompt-only generations
and reference-conditioned edits, including character sheets and storyboards.

The backend uses `https://api.openai.com/v1` and `gpt-image-2` by default.
An operator can set `OPENAI_BASE_URL` to a trusted HTTPS base URL including its
API version path, and `OPENAI_IMAGE_MODEL` to a GPT Image model supporting the
requested dimensions. Configure these values in the Compose environment and
recreate the backend. Set `VIDEOFLOW_DISABLE_CODEX_IMAGES=1` if the selected
Codex installation cannot run its native image tool. Text planning still uses
the selected local session.

An uncertain or failed native generation stops for review to avoid a second
image call. OpenAI API usage can incur charges. Settings checks configuration
without generating an image.

### Google Gemini images (Nano Banana)

Save a Google AI Studio key under **Settings → AI connections → Google Gemini**,
or provide `GEMINI_API_KEY` to the backend, then choose **Google Gemini** as the
picture service in **Settings → AI setup**. The picker lists OpenAI, fal.ai,
OpenRouter and Google Gemini; the service chosen there is used whichever keys are
saved. With none chosen, a saved OpenAI key decides. Pictures still come from
Codex when the conversation runs on Codex.

The model picker next to the service offers Nano Banana Pro
(`gemini-3-pro-image`, the default, up to 14 reference pictures), Nano Banana 2
(`gemini-3.1-flash-image`, up to 14) and Nano Banana (`gemini-2.5-flash-image`,
up to 3; Google plans to retire it no earlier than 15 March 2027). Requests go
to `https://generativelanguage.googleapis.com/v1beta` with the key in the
`x-goog-api-key` header. Pictures are requested at Google's default 1K size and
cost about $0.134, $0.067 and $0.039 each respectively. Settings checks
configuration without generating an image.

### Google Gemini video (Gemini Omni Flash)

The same Google Gemini key also makes scene videos. In **Settings → AI setup →
Video**, choose the model **Gemini Omni Flash (Google)**. Google Gemini then
appears as its only service. It shows "(no key saved)" until the key is saved.
Google no longer offers Veo through the Gemini API: the Veo 3.1 previews shut
down on 22 October 2026, and Google names `gemini-omni-1.2-flash` as the
replacement. That is the model VideoFlow uses.

Clips go to the Interactions API (`POST /v1beta/interactions`) as a background
interaction. The key travels only in the `x-goog-api-key` header. VideoFlow then
polls `GET /v1beta/interactions/{id}` every few seconds, within the usual
30-minute job limit, and saves the finished MP4. When Google keeps the clip in
its Files API instead, it is downloaded from Google's API host with the key in
the header. A redirect to any other host is followed without the key.

What a clip can be:

| Setting | Gemini Omni Flash |
|---|---|
| Length | 3–15 seconds; 3–10 seconds with a first and last frame |
| Shape | 16:9 or 9:16 |
| Size | 360p, 720p (default), 1080p or 4K (the last two are upscaled by Google) |
| Pictures | a first frame, a first and last frame, or up to 6 reference pictures |
| Sound | always on; a silent scene is asked for silence in its prompt |

A seed, voice recordings, other shapes or sizes, and longer clips are refused
before anything is sent.

Google bills the video as output tokens at $12.00 per million. The quote uses
Google's own per-second rate: about $0.023 at 360p, $0.070 at 720p, $0.104 at
1080p and $0.209 at 4K. The input prompt and pictures add well under a cent per
clip and are left out. Prices were checked on 23 November 2026.

### Google Gemini voices

The Google Gemini key also speaks narration and character lines. Choose
**Google Gemini** as the voice service in **Settings → AI setup → Voice**, or per
character in **Voices**. It shows "(no key saved)" until the key is saved. The
voice list is Google's 30 prebuilt voices, such as Kore (firm), Puck (upbeat)
and Sulafat (warm). Searching filters them by name and style, and choosing one
asks Google nothing. Google detects the language from the text.

Speech uses `gemini-3.8-flash-tts` (the default) or `gemini-3.8-flash-lite-tts`
through the Interactions API, with the key in the `x-goog-api-key` header. The
2.5 and 3.1 preview TTS models shut down on 18 December 2026 and are not offered.
Google returns WAV, which VideoFlow converts to MP3 with FFmpeg, as it keeps
every other voice line. Lines are cached by voice and text, so an unchanged line
is never paid for twice.

Google bills speech per million tokens: $0.50 for the text and $9.00 (Flash) or
$6.00 (Flash-Lite) for the audio, at 25 audio tokens per second. Both prices
double on 1 January 2027, and the cost page uses the price in force on the day
the line was made. A ten-second line costs about $0.002. A character's voice
sample can be spoken by a Gemini voice, the same as with the other services.

### Google Gemini listening (caption timing)

Captions on clips with their own sound are placed where the words are actually
heard. VideoFlow listens to the clip once and remembers the answer. **Settings →
AI setup → Listening to audio** chooses who listens: OpenAI (`whisper-1`) or
Google Gemini (`gemini-3.5-transcribe`, with word-level timestamps, on the
Gemini key). A service without a key shows "(no key saved)". With none chosen,
OpenAI listens when its key is saved, and otherwise Google Gemini does.

Gemini receives the clip's audio inline as WAV through the Interactions API,
with the key in the `x-goog-api-key` header. The script still supplies the
words, and only the times are used. Google notes that word timestamps can lower
transcription accuracy, and that they cannot be combined with a vocabulary
hint, so none is sent. It costs about $0.005 per minute of audio (Google's
estimate of $0.003 in plus $0.002 out).

Looking at pictures still uses OpenRouter. The picture checks rely on
OpenRouter's structured answers, so moving them to Gemini would need a separate
change.
