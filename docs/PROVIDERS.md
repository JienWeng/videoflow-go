# Provider routing

This guide describes the provider architecture on current source. The v0.3.2 browser preview predates the fal/BytePlus integration; rebuild current main or use a later release that includes it.

| Task | Fresh-install default | Other supported provider |
| --- | --- | --- |
| Story, chat, shot/prompt agents | OpenRouter | Named OpenRouter connections |
| Vision/QA | OpenRouter | Named OpenRouter connections |
| Transcription | OpenRouter | — |
| Characters, props, storyboards | fal.ai | OpenRouter |
| Scene/shot video | fal.ai | OpenRouter or BytePlus ModelArk international |

## Set up and switch

Open Settings → Providers. Save an OpenRouter key for text/vision/transcription and a fal.ai key for images/video. Save a BytePlus ModelArk key if you want direct Seedance video. Keys are sent only by the local backend. Environment alternatives are `OPENROUTER_API_KEY`, `FAL_KEY`, and `BYTEPLUS_API_KEY`; their presence is not proof of model access.

In Settings → Engines, select Image provider and Video provider independently, then choose a model from that provider's supported catalog. Switching providers restores its previous model choices. Image/reference model IDs cannot be sent to a video-only provider, and a model ID from one provider is rejected under another. Text-agent selectors include only OpenRouter connections.

Media **Check configuration** verifies the locally saved configuration, not the account's permissions or a paid generation. Media **Verify** checks the supported model profile without submitting generation. OpenRouter's text-model verification is a small billable completion. A later small, budgeted generation is needed to verify real account/model access and output quality.

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

Guided creation captures its media routes before beginning paid stages. Later Settings changes affect new requests, not that pipeline or a submitted video job. Provider request IDs are stored before polling. Re-render is an explicit new paid request on the original route. Unknown submission results stay fenced for reconciliation; the app never automatically submits to another provider as a fallback.

My videos shows the saved job/provider and can resume a verified existing provider ID. This resumes polling only. fal's queue status `COMPLETED` is followed by result retrieval; the result can still report failure. BytePlus task states map to the app's pending/running/completed/failed states. Saved outputs continue through the existing editor, caption burn, assembly and download paths.

Provider keys are not attached to output downloads, and production output URLs cannot reach local/private services. Selected local references are encoded for the generation request; localhost storage URLs are not sent as if they were publicly reachable.

## Verification

Local tests use separate counting provider mocks, real FFmpeg and browser checks. They verify auth/payload translation, defaults and migration, provider/model switching, reference conditioning, pipeline/worker route snapshots, failure/recovery behavior and downloadable outputs. No paid generation is used by these tests.

Official references: [fal queue API](https://fal.ai/docs/documentation/model-apis/inference/queue), [fal Seedance image-to-video schema](https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video/api), [fal image-edit schema](https://fal.ai/models/fal-ai/nano-banana-2/edit/api), [BytePlus creation API](https://docs.byteplus.com/en/docs/modelark/create-video-generation-task-api), [BytePlus retrieval API](https://docs.byteplus.com/en/docs/modelark/get-video-generation-task-api).
