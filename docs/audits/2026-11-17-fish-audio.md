# Fish Audio character speech

The reported render error refers to the Shopkeeper's missing saved voice profile. Adding an API key does not itself assign a voice: each speaking character needs an explicit saved voice ID. Existing voice identity checks remain.

## Implementation
- Settings → AI connections includes Fish Audio, a masked backend key and an official, fixed endpoint. Checking configuration does not contact Fish or synthesize speech. Optional FISH_API_KEY environment configuration is supported and stripped from native agent environments.
- Voice & dialogue can bind each character to Fish Audio or the existing fal/MiniMax route. Fish supports saved voice model IDs and explicit S2.1 Pro, S2.1 Pro free-tier or S2 Pro selection. Fish voice cloning/model creation is handled in Fish Audio; no cloning endpoint is silently substituted.
- Approved line speech uses one synchronous POST /v1/tts with Bearer authorization, the explicit model header, reference_id, text and MP3 format. Sources: https://docs.fish.audio/api-reference/endpoint/openapi-v1/text-to-speech.md and https://fish.audio/developers/ (checked 2026-11-17).
- The existing fingerprint and task-claim fence prevent duplicate submission. Uncertain requests do not automatically retry and cannot be reconciled as fal queue receipts. Audio is validated locally and saved as a project asset with script, speaker and profile bindings; it can be selected by the existing lip-sync workflow.
- Scene storyboard/render preparation checks voice setup before generating a replacement storyboard. Provider disclosures stay open while entering credentials. Saved voice/dialogue forms clone snapshots of reactive state so existing documents reload correctly.
- Missing voice errors identify the character and include a character-specific Voice & dialogue URL; chat provides a setup button.

## Verification and limits
Full Go suite and focused provider/workflow tests run against fake providers. Tests cover auth/model/payload, unknown models, provider errors, no preparation-time payment, saved audio provenance, duplicate fences and missing fal credentials on Fish speech. UI check reports zero errors/warnings; mechanical design scan reports no findings. Browser checks passed with fake keys and mocked APIs: six desktop/mobile checks, including saved-profile reload, and two accessibility scans with zero violations and no browser errors. Results are saved alongside this report.

No live Fish, Codex image or video generation was submitted. Actual voice availability/account access and synthesis quality require the user's configured account. Fish output is a separate speech asset; this change does not automatically dub or lip-sync the current video or approve character voice profiles.
