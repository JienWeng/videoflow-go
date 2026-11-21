# Simplify AI sessions — implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan inline, followed by one read-only review of the completed change.

**Goal:** Make choosing an AI session and understanding the next action easy, without removing session history, reference selection or recovery controls.

**Architecture:** Keep the existing director APIs and the current visual system. Replace the dense two-column card layout with a compact session picker, a selected-session area and progressive disclosure. Use one small tested presentation helper to prevent an installed executable or a saved session from being described as authenticated and ready.

**Tech stack:** Svelte 5, existing Button components, semantic HTML details/summary, Node test runner.

**Spec:** The user approved the preceding proposed simplification: compact AI status, selected session and next step first, references expandable, history and technical details collapsed.

## Constraints

- Preserve existing data and all create/select/save/cancel/acknowledge APIs.
- Missing CLI: say setup is needed; saved status must never say ready.
- Detected CLI: explain that sign-in is checked on the first prompt.
- Running and interrupted turns retain explicit cancellation/review gates.
- Keep current fonts and theme; scope edits to the sessions page and its presentation helper/tests.
- Never automatically submit inference or replay past turns.

## Implementation and checks

- [x] Add failing state-presentation tests for missing/unknown engines, detected engines, running sessions, review-required sessions and disabled local routing.
- [x] Implement the status helper and pass the tests.
- [x] Rewrite the sessions page: compact picker, inline New session form, honest status and one primary CTA; collapsed setup, references and history; saved reference summary and visible unsaved-change state.
- [x] Keep recovery warnings visible and require inspecting history before acknowledging an interrupted turn. Preserve edits while opening disclosures, and prevent switching or continuing with unsaved references.
- [x] Run Svelte checks, Node tests and production build.
- [x] Verify actual desktop/mobile layout, missing-engine and created-session states, reference save, disclosure controls, keyboard focus and clear next action. Exercise detected/running/review states with deterministic browser fixtures if available, clearly separating fixtures from live inference.
- [x] Obtain one read-only code review, address material findings, rebuild the Docker frontend and verify the Tailscale endpoint.

## Execution notes

The existing checkout includes earlier authorized QA fixes; preserve them. No branch reset, dependency upgrade, typography replacement or paid calls are part of this work.


## Verification results

- Status presentation: six Node tests passed after first reproducing the misleading ready states.
- Svelte check: zero errors and zero warnings. Production Docker frontend build passed.
- Updated `scripts/e2e_director.mjs`: 14 fixture-backed browser checks passed (seven each at 1440 × 1000 and 390 × 844), zero browser runtime errors. Covered creation, selected/unknown/missing engines, saved references and discard, running cancellation, interrupted recovery, detail-load failure, long names, overflow and backend failure.
- The browser test first reproduced a recovery path that discarded unsaved references. Acknowledgement now has both a handler guard and a disabled button while edits are unsaved; the same browser test passes.
- The existing browser regression script was run with its documented Playwright workflow and `CHROME_BIN` pointed at the installed Chromium binary. Fixture tests do not call real providers.
- Native T3 browser inspection of the live app: desktop/mobile layouts, collapsed defaults, setup disclosure focus, New session focus, reference edit/discard and truthful missing-engine status checked. Existing sessions and references were preserved.
- One read-only code review identified the recovery guard issue above; it was fixed and retested. No backend API changes or paid inference were part of this simplification.
- Impeccable scan of the changed page: no deterministic findings. Existing typography was retained; no font ignore was added.
