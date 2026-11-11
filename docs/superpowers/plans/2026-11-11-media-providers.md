# Selectable media providers implementation plan

> For agentic workers: use superpowers:executing-plans for inline implementation, one final independent review and one verified fix pass.

Goal: add fal.ai and direct BytePlus Seedance end to end, fal default media, easy correct provider/model selection.
Architecture: Go media interface and provider-specific adapters; routing resolves credentials and model choices separately from text; durable jobs retain their resolved route. Existing app/captions/storage remain shared.
Tech stack: existing Go HTTP/SQLite and Svelte; no new application dependencies.
Spec: docs/superpowers/specs/2026-11-11-media-providers.md.

Global constraints: personal repository only; no paid calls, no secret logging, no automatic provider fallback; preserve existing data/jobs and explicit settings. All tests use local mocks; real FFmpeg verifies exports.
Review focus: model-ID/provider mismatches; ambiguous create responses; old jobs after settings change; incompatible reference modes; credentials leaking through result/status URLs.

- [ ] 1. Provider contract/adapters: internal/providers/media.go, fal.go, byteplus.go and tests. Write failing exact-auth/schema/status/result tests, implement adapters with strict catalog capabilities and shared bounded HTTP helpers. RED→GREEN provider tests.
- [ ] 2. Routing/config migration: internal/routing/media.go plus tests, config env keys, settings model preferences and DB migration. RED→GREEN defaults/legacy override/model mismatch tests. Snapshot provider endpoint into persisted jobs.
- [ ] 3. Generation/worker integration: API generation/preflight/render/scenes/videos/recovery and worker call the media client; text/transcription stay OpenRouter. RED→GREEN counted mock routing, restart, setting-change and retry tests. Preserve durable job fences.
- [ ] 4. Settings UI: expose three provider cards, capability-filtered selectors and provider-specific model preferences; remove forced OpenRouter copies/branches and unsupported task choices. Svelte check/build plus browser settings/guided workflow with mocks.
- [ ] 5. Verification/docs: update llms, user/install/env docs and tests to show actual routing and migration; full tests/race/vet/check/build, mocked end-to-end and native packaging compatibility. Fresh independent whole-change review; fix Important findings with RED→GREEN tests; push personal main only.
