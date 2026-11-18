# Storytelling and production library implementation plan

Goal: Separate narrative direction from visual style, surface reusable production assets, and retain cost-aware segment rendering in the existing workflow.

Architecture: Add optional server-owned storytelling IDs to the existing brief and saved scene snapshot. Resolve guidance on the backend; preserve legacy empty selections. Extend Library filters using existing asset types, version groups and exclusion metadata. Existing grouping remains authoritative: shots are not automatically paid calls.

Approved approach: Extend brief, Library and scene planning, as requested after the ToonFlow study. Do not create parallel pages or copy ToonFlow code. The unidentified alien example is outside this implementation.

- [x] Add tested narrative catalog, brief validation, chat merge and saved prompt propagation.
- [x] Add compact independent storytelling selection and confirmation summary.
- [x] Add production readiness/version filters and improve reference-board guidance, preserving reuse and explicit scene bindings.
- [x] Run Go, Svelte and mocked browser checks; review changes, update PR #2 and deploy when verified.

Sources: HBAI-Ltd/Toonflow-app master packages/skills/workflow/references/assets.md; packages/skills/v118workflow/references/knowledge/visualStyles.md and narrativeGenres.md; packages/skills/canvas/references/videoProduction.md. Adapt concepts; no code copied. No paid provider tests.
