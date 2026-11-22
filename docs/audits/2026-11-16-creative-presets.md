# Creative presets and community packs — 2026-11-16

Adds Clay stop-motion, Layered paper-cut, Graphic novel and Soft 3D animation alongside Picture book and Cinematic. Each style supplies distinct story/shot direction, image treatment and motion guidance. Explicit selections override the project look for a new story; Project style remains the default when configured. Resolved creative guidance is persisted with scene briefs and generated assets, including later scene studio generation and asset versions. Pack removal/replacement cannot restyle saved work. Native Codex-first routing and review gates are unchanged.

Settings → Creative presets supports bounded UTF-8 JSON preview, explicit install/replace, export and removal. Packs are data-only extensions, not executable plugins or a hosted marketplace. Namespaced IDs protect built-ins. Strict schema/version/field checks and compare-and-swap storage prevent invalid or concurrent updates. Attribution is author-supplied. Downloadable starter pack, JSON schema and authoring guide are included.

## Evidence

- Full Go test suite passed, including pack validation/lifecycle, explicit-vs-project style and removed-pack snapshot regressions.
- Svelte check: zero errors and warnings. Production frontend/backend builds verified separately at deployment.
- `scripts/e2e_creative_presets.mjs`: 22 checks pass at 1440px and 390px; four WCAG A/AA axe scans, zero violations; zero page errors. Result IDs/evidence are in the adjacent JSON report.
- Browser checks cover malformed JSON recovery; preview without install; namespaced install; export round trip; idempotence; reviewed replacement/conflict; selector/prompt disclosure; viewport fit; removal; unchanged fake provider counters.
- Independent review identified saved brief re-resolution and missing studio asset context. Both fixed; re-review found no further important defects.
- Existing progress and canvas layout unit checks pass. No live image/video generation was performed; visual output quality and creative adherence remain model-dependent and unverified with paid providers.

## Try it

Create story → Visual style. Settings → Creative presets → Download starter pack, edit its ID/name/prompts, preview and install. Use `.videoflow-presets.json` files to exchange packs. The bundled format guide documents limits and API behavior.
