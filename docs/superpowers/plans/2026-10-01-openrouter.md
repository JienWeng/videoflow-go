# OpenRouter implementation plan

Goal: All AI work uses OpenRouter with saved credentials, current model catalogs, and real outputs. Existing local projects/assets remain intact. User authorized implementation directly.

Architecture: A shared routing resolver reads SQLite settings at call time. One OpenRouter HTTP adapter handles authenticated requests, model capabilities, structured text/vision, images, and asynchronous videos. Existing API routes and background operations remain the frontend contract.

- [x] Add failing provider contract tests: HTTP errors, structured JSON errors, image decoding, video unsigned URLs and authenticated content download, parameter validation.
- [x] Implement the adapter and dynamic credential/model resolver; validate modalities against live catalogs before generation.
- [x] Add failing API tests for saved keys, real connection/model checks, discovery, provider restrictions, and agent model routing. Implement settings and runtime resolution.
- [x] Add failing worker tests for real submission/poll/download, failed jobs, cancellation, and restart without duplicate submissions. Replace simulated render/QA results.
- [x] Connect character references, storyboard/scene assets, frame QA and image recognition to OpenRouter; remove deterministic agent fallback responses.
- [x] Limit settings UI to OpenRouter, load modality-specific catalogs, replace invalid defaults, and update documentation.
- [x] Run Go tests/race checks/vet and frontend checks/build. Verify live authentication/catalog access without generating paid media. Report any remaining external verification honestly.

Review focus: omitted key must preserve saved key; persisted overrides must work after restart; unsupported model inputs fail before paid submission; download auth must never reach an unrelated host; resumed videos reuse provider job IDs and remain resumable after shutdown.
