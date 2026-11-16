# Codex-first image generation — 2026-11-16

## Routing

With a local Codex session selected and its CLI image_generation feature enabled, character sheets, props, locations and storyboard generation now try the built-in image_gen tool before selecting an image API client. Planning remains restricted to text proposals with the image tool explicitly disabled. Image generation uses a separate ephemeral Codex turn with shell/search/apps disabled and no image-provider API credentials. The selected planning conversation's native ID is retained.

The backend stages approved/current input images in a private temporary workspace, preserving their PNG/JPEG/WebP format. The native turn requests one image with the production aspect ratio, style and identity constraints. Import accepts only a newly created, nonsymlink raster beneath CODEX_HOME/generated_images, within file and dimension limits, and fully decodes it before copying it into normal asset storage. Assets retain the existing pending approval and frozen identity metadata.

Image APIs are used only when capability is absent or the backend identifies unsupported input before starting the native turn (more than five references or unsupported native input format). No API fallback follows a native turn, including an assistant-reported unsupported result: the image tool may already have been attempted. Authentication, quota, timeout, invalid output and context-change failures stop for review. Existing saved media routes remain the API fallback. Video generation continues through the configured video provider. Local pipelines allow up to 45 minutes for multiple sequential assets; individual native image turns remain bounded at eight minutes.

Create story displays Codex-first routing and fallback policy. Asset metadata records the actual provider/model and any API fallback reason. Native-image preflight skips image API requirements/catalogue checks while retaining local-session and video requirements.

## Evidence

- Installed backend Codex CLI 0.169.0 reports `image_generation stable true`; checking this capability does not invoke a model.
- Full Go suite passed; focused tests passed after final reference-format handling.
- Native-success fixture imports a valid image, preserves the planning native ID, keeps review pending, receives reference arguments, strips OPENAI_API_KEY, and makes zero image-provider requests.
- Native unsupported/failed/protocol-failure fixtures make zero image-provider requests. A backend-detected six-reference request uses the configured API and records its fallback reason.
- Fresh-path confinement, stale/arbitrary file rejection and truncated PNG rejection tested. PNG/JPEG/WebP full raster decoding uses golang.org/x/image for WebP support.
- Native preflight fixture confirms no image API requests and successful video/session checks.
- Svelte check: zero errors/warnings.
- Production frontend fixture: 22 desktop/mobile checks and four axe scans passed, including the Codex-first copy, combined reference review, safe removal, approval gating and continuation. Adjacent JSON contains check evidence.
- Independent review identified ambiguity in runtime unsupported fallback and weak signature-only raster validation; both were fixed and re-reviewed with no further important findings.

## Live verification limit

The selected production Codex session remains review_required after an earlier interrupted turn. Its review acknowledgement was not cleared or bypassed. No live native image generation, image API call or story retry was submitted. Successful fixture routing and installed CLI capability are verified; account-level end-to-end image generation still needs the user's session review followed by one real generation.
