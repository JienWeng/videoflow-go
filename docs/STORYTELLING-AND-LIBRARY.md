# Storytelling methods and production library

In Create story or the chat's creative brief, open **Look & mood**. Choose the visual look separately from **Storytelling method**:

- Character drama: character goals, dialogue, relationship changes and motivated reaction shots.
- Adventure & discovery: clear goals, geography, discoveries and payoff.
- Guided explainer: a recurring guide where supplied, concrete examples and a concise takeaway; no invented factual claims.
- Visual comedy: readable setup, contrast and payoff.
- Follow my story: no additional narrative recipe; also the legacy default.

The optional storytelling_id is preserved through chat follow-ups, request confirmation and saved scene generation_brief. The backend resolves trusted guidance; unknown IDs fail before generation. Changing it invalidates UI confirmation, as with other brief fields. It does not change visual style, providers, voice mode, duration or permission to spend. Existing compatible-shot grouping remains authoritative; it does not render every shot separately.

Library → Media → **Production filters** starts collapsed. Filter saved assets with a file, missing media or excluded references, and optionally show only the newest file in each existing variant group. Clear filters restores all versions. These are read-only collection views: they never replace scene bindings, delete old assets or submit generation. “Has file” reflects the saved file association, not a filesystem health guarantee; generation still validates media availability.

New character references request a head close-up and consistent front/side/back full-body views adapted to the species. New prop references isolate the object; location references show a readable empty space. Suitable existing assets remain reusable. Generation scaffolding is not saved as the reusable prop/location description, preserving cross-scene matching.

## Verification

- Full Go suite passed (including cross-scene generation deduplication).
- Svelte check: 0 errors, 0 warnings; production frontend/backend builds.
- scripts/test-production-library.mjs: pure brief/readiness/version contracts.
- scripts/e2e_creative_brief.mjs: desktop 1440px and mobile 390px; method survives follow-ups/reload and reaches confirmed generation payload; edits require fresh confirmation; zero axe violations.
- scripts/e2e_production_library.mjs: both widths; collapsed default, latest-version filtering, missing/excluded media, clear/reset, no write calls, no overflow/browser errors, zero axe violations.
- Providers mocked; no paid generation or image-quality claim.

## Sources and boundaries

Concepts adapted from the official ToonFlow guides (no source code copied):
- https://github.com/HBAI-Ltd/Toonflow-app/blob/master/packages/skills/workflow/references/assets.md
- https://github.com/HBAI-Ltd/Toonflow-app/blob/master/packages/skills/v118workflow/references/knowledge/visualStyles.md
- https://github.com/HBAI-Ltd/Toonflow-app/blob/master/packages/skills/v118workflow/references/knowledge/narrativeGenres.md
- https://github.com/HBAI-Ltd/Toonflow-app/blob/master/packages/skills/canvas/references/videoProduction.md

The specific “Alien Takes You to See the World” example was not identified. Guided explainer is a general method, not a recreation of that example. Narrative methods are built-in; existing community JSON packs continue to provide visual styles and per-stage guidance.
