# Creative brief verification — 2026-11-17

The new-story flow gathers the original idea, visual style, tone, audience, destination platform, aspect ratio, video size, language, speech mode, total duration and scene count. Chat asks for missing groups; one editable brief is shared with Create story. The final summary and explicit confirmation precede Create. Edits invalidate confirmation. Project defaults remain separate.

## Coverage

- BRIEF-01: API regression verifies edited follow-up choices, original idea, trusted style fingerprint and no render side effects.
- BRIEF-02: Contract tests reject incomplete/unconfirmed choices and changed style fingerprints. Legacy API callers and saved story snapshots remain compatible.
- BRIEF-03: Draft tests enforce exact total duration, scene count, aspect, speech mode and language metadata. Render regression checks saved resolution and creative direction reach the job.
- BRIEF-04: Real isolated-router browser workflow selects visible project preset cards and creates fake references through the shared story brief without adding image approval stops.
- BRIEF-05: Mocked browser coverage verifies edited choices travel to chat, only one active brief remains, reload restores the draft and confirmation must be renewed.
- BRIEF-06: Desktop (1440px) and mobile (390px) guided flow: 14 checks, six zero-violation WCAG A/AA scans and no browser errors. Dedicated brief flow: eight behaviors per viewport and two additional zero-violation WCAG scans. Go suite, Svelte check and production builds are required before deployment.

## Reproduce

- `node --test frontend/src/lib/brief/brief.test.mjs`
- `go test ./... -count=1` with FFmpeg available.
- In frontend: `npm run check` and `npm run build`.
- Start the opt-in `TestE2EBrowserFixture` with `VIDEOFLOW_E2E_SERVE=1`. Serve the production frontend at port 8091, proxying `/api` to fixture port 18091.
- `node scripts/e2e_guided_chat.mjs`
- `node scripts/e2e_creative_brief.mjs` (application/provider responses mocked).

No live paid generation was used. Outputs are under `/tmp/creative-brief-*` and `/tmp/videoflow-no-review`. These checks prove parameter handling and UI behavior; model artistic quality, literal translation, native speaker fidelity and platform acceptance still require inspecting generated results.
