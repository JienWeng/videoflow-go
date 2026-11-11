# Media provider implementation verification

Repository: JienWeng/videoflow-go. Design and task plan: docs/superpowers/specs/2026-11-11-media-providers.md and docs/superpowers/plans/2026-11-11-media-providers.md.

Implemented: fal default new media routes, OpenRouter text/vision/transcription, selectable direct international BytePlus video, curated exact API profiles, provider-specific model preferences, migration preserving existing routes, durable job endpoint/provider/model snapshots, route capture shared by preflight and generation, provider-aware retry/reconciliation, capability filtering, reference/first-frame translation, credential-free bounded output downloads and private-network blocking. No application dependencies were added.

## Evidence

- Full `go test -race ./...` and `go vet ./...` passed after the review fix.
- Svelte check: zero errors/warnings. Production build and two chat-action Node tests passed.
- Electron non-GUI tests: five passed; one packaged-GUI test skipped because the packaged runtime is not installed for that test. Package safety and four payload-preparation tests passed; native complete-bundle tests await the native release build.
- Separate counting mock servers cover fal image + fal video and fal image + BytePlus video. They assert credentials and provider-specific payloads, character references, language and first-frame conditioning, settings changes during preflight and generation, and persistent downloadable output.
- Worker tests prove job routing persists after defaults change. Endpoint pinning does not reopen a durable submission claim. Provider/model mismatches are rejected; legacy image/video/reference-only model overrides preserve OpenRouter routes.
- Browser fixture switched fal→BytePlus→fal and restored fal's saved model, checked truthful media configuration status, generated two clips, selected outputs across galleries, assembled and opened the editor with no page errors. Native preview was unavailable due to the host's AppArmor sandbox block; isolated local browser automation was used with provider mocks.
- No paid provider calls or real-account entitlement/fidelity/Chinese speech tests were performed.

## Independent final review and fix pass

Fresh independent review found no Critical findings. One Important finding covered fal result 401/403/404, malformed/missing fal results and BytePlus success without an output URL being labelled failed, which could permit another paid generation. All six regression cases were observed failing, then passing after adapters retained a nonterminal retrieval error. The worker now preserves the existing task ID and reconciliation fence. The full race suite passed afterward. No second review was used.

Deferred Minor: an explicit advanced BytePlus render request can still forward `seed` although the selected Seedance 2.0 profile does not document that input. Current UI and guided requests do not supply it; omit it for direct BytePlus API calls. fal seed documentation is contradictory between overview and input schema, so its live behavior is not claimed verified.

## Decisions and tradeoffs

1. The user's explicit routing/provider selections are the design authority; this implementation records that selected design without an additional approval round. Newly discovered product choices still require clarification.
2. Work stays in the dedicated personal-repository checkout, preserving the organization checkout. The thread's original project directory still names the organization workspace; commands and pushes must explicitly target the personal checkout/remote.
3. Initial models use explicit documented profiles. Reference requests on the fal image base profile select its documented `/edit` sibling. Unknown endpoints require a supported profile and tests, rather than assuming every model uses the same schema.
4. Media configuration/catalog checks do not claim paid execution or real account/model access; a later small budgeted live generation remains necessary.
5. Real billing, output fidelity, actual account entitlement and speech quality remain unverified without live calls. Native packaging is a separate build gate; older v0.3.2 ZIPs do not gain new providers by refreshing the app.

Final UI verification also caught a stale “Connected” result badge after a media configuration-only check. A browser probe failed before correction and passed after the badge became “Configuration checked” without a network-latency claim. Frontend check/build were repeated for this copy fix.
