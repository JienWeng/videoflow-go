# Settings checklist refinement

Reference: https://www.checklist.design/web-app/settings

Scope: existing local single-user Settings, preserving the app's theme and capabilities.

- Structure: General has appearance, scene defaults and captions; AI connections has credentials; Media models has generation defaults; Advanced has render prompts, agent routing and recovery. Existing provider/engine/agent hash links resolve to their relevant sections.
- Preferences: theme is elevated, has keyboard radio navigation, and remains device-local. Other defaults save on change with persistent live save/failure feedback. Failed saves restore the last confirmed form values. Saved transcription IDs and legacy dialogue language values remain visible even when absent from the current catalog.
- Credentials: keys stay masked; removal requires a separate confirmation explaining effects. Bulk routing reset also requires confirmation. Paid test controls still require explicit actions.
- Recovery: initial load errors offer retry and hide unusable controls. No generation or credential mutation was performed on the live instance.
- Account, password, notification and billing features do not exist in this local app; no placeholder controls were added for these checklist items.
- Presentation: flat sections and separators replace repeated card framing; mobile categories use a two-column grid and selects fit their container.

Verification: `npm run check --prefix frontend`, frontend Docker production build, `scripts/e2e_settings.mjs` using the repository's mocked Playwright workflow on desktop/mobile, and native T3 live inspection. See task response for final outcomes.
