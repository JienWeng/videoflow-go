# Live speech catalogs, matching, and primary provider

## User behavior
- Voice & dialogue loads official speech model metadata when a Fish Audio or ElevenLabs picker opens. A persisted 24-hour cache avoids repeated metadata requests; Refresh models checks immediately. No scheduler runs while the application is closed. A failed refresh retains a clearly marked older catalog with its original successful-check timestamp.
- Fish engine IDs/default come from the official TTS OpenAPI schema. Its /model endpoint lists voices, not speech engines. ElevenLabs /v1/models supplies TTS capability and language metadata; non-TTS and alpha-only entries are excluded. Its documented stable default is preferred when present; otherwise Automatic chooses a compatible returned entry. Catalog order is not represented as a reliable release-date ranking or a promise that the newest model is best.
- Users can choose an API-returned model or Automatic. Automatic resolves when preparing speech. The resulting task pins the actual model, endpoint, approved voice and source revisions; metadata refreshes do not change or rerun prepared tasks.
- Voice browsing uses Fish's licensed voice library or ElevenLabs' account-accessible voices, with search, language and pagination. Auto-match fills a suggested voice ID using deterministic overlap with character descriptions, personality and voice rules. The explanation names matching metadata; a fallback explicitly says no descriptive match was found. Matches are limited to the returned filtered candidates, not a quality guarantee or a claim that every library voice was auditioned. Saving remains explicit, and no synthesis happens during matching.
- Primary speech provider is selectable from Settings or Voice & dialogue and shown with a badge. It applies to new character voice setup, including pristine unsaved forms, while existing saved voices and user drafts remain unchanged.
- ElevenLabs uses its own masked key, fixed official endpoint, xi-api-key authentication, POST /v1/text-to-speech/{voice_id} and an explicit model_id. It shares the existing synchronous speech artifact importer and durable duplicate-submission fence. Keys are stripped from native CLI environments; no automatic provider fallback or cloning was added.

## Sources checked 2026-11-17
- https://docs.fish.audio/llms.txt
- https://docs.fish.audio/api-reference/openapi.json
- https://elevenlabs.io/docs/api-reference/models/list
- https://elevenlabs.io/docs/api-reference/voices/search
- https://elevenlabs.io/docs/api-reference/text-to-speech/convert

## Verification
Full Go tests and Svelte checks run. Provider/API regressions cover dynamically returned future model IDs, explicit TTS filtering, catalog caching and refresh, stale-cache warnings during outages, Fish schema/voice separation, pagination, deterministic voice matching, primary preference persistence, pinned-model ElevenLabs synthesis and secret isolation. Browser workflows passed eight desktop/mobile catalog/provider checks and six Fish Audio regression checks, with four clean accessibility scans and no browser errors. Workflows use mocked catalogs, fake keys and generated-audio responses; no live generation is submitted. Speech history identifies the actual provider rather than labelling every task as fal.

Actual account catalog/voice access and synthesis quality require configured provider credentials. Fish/fal backward compatibility remains supported. This change refreshes speech catalogs; it does not replace the image/video provider catalogs.
