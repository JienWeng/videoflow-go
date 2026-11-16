# Draggable assistant and right docking — 2026-11-16

The user clarified that the assistant must be a movable window with the option to stick to the right side. Studio now supports both behaviors.

## Behavior

- Start docked, reserving canvas space. Drag the titlebar background or grip to automatically float and move the window. The Float / Dock to right button also switches modes.
- Floating mode overlays the full canvas; docked mode restores the reserved right column. Closing, reopening, dragging and docking keep chat, history and inspector drafts mounted.
- Clamp position inside the Studio workspace during drag, resize and viewport changes. Leave vertical movement room on laptop screens. Resize from the left edge while keeping the right edge anchored; retain expand/restore.
- Remember mode, position and desktop width in browser-local storage. Ignore malformed layouts and clamp out-of-range coordinates. Mobile keeps the full-width pane without overwriting the preferred desktop width.
- Support arrows, Shift+arrows and Home/End on the move handle. Session/menu/close controls do not initiate a drag. Pointerup, cancellation and lost capture release listeners.

## Verification

| Suite | Expected | Passed |
| --- | ---: | ---: |
| Window movement, automatic undocking, docking, pointer cancellation, bounds, resizing, storage and mobile | 18 | 18 |
| Workspace and draft regressions | 48 | 48 |
| Chat history/retry, proposals, specialist review and cancellation | 16 | 16 |
| Canvas editing, context, resize and chat submission | 26 | 26 |
| Character/asset dialogs and branding | 15 | 15 |
| Total browser checks | 123 | 123 |
| Automated WCAG A/AA scans | 9 | 9 |
| Geometry/parser assertions | 10 | 10 |

Zero page errors and zero automated accessibility violations. Every browser check has an ID in [the results file](2026-11-16-draggable-assistant-results.json). Remaining IDs: none. Geometry tests were observed failing before the helper existed, then passing after implementation.

Independent review found a minor desktop-width loss during mobile transitions. The observer now preserves the preferred desktop width, with a browser round-trip assertion. Follow-up review confirmed the correction and automatic titlebar undocking; no important findings remained.

Svelte check: zero errors/warnings. Docker production frontend build passed. All browser suites ran against that image with fixture API interception. The frontend alone was deployed; backend data and native authentication volumes were retained. Native shared-browser interaction verified real dragging/automatic undocking and return to right docking on the deployed app. Local HTTP and Tailscale HTTPS returned 200.

No live native inference or paid provider generation was submitted. Real Safari/iOS, screen-reader operation and live model quality remain unverified. No new detector suppressions or dependencies were introduced.
