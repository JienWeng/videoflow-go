# Reference-led floating assistant — 2026-11-16

The user requested a closer visual match to the supplied floating-chat screenshot after reviewing the unified workspace. The previous edge-to-edge workbench remained visually heavy.

## Changes

- Inset assistant surface with 16px corners, restrained elevation and a compact conversation-selector header. No extra Chat-only tab bar; Details/Chat tabs appear for selected items.
- Close/reopen without unmounting chat or inspector; preserve unsent prompts, selected context, history and dirty forms. Return focus to the opener on close and the active editor/composer on reopen.
- Reserve space beside the floating panel so nodes and canvas controls remain clickable. Closing returns that space to the canvas. Preserve pointer/keyboard resize and expand/restore.
- Welcome content at the top, custom VideoFlow mark and three fill-only starter cards. Integrated rounded composer, quieter Planning control and no extra horizontal separators. No unsupported attachment, microphone or model tools were added.
- Mobile keeps its single-pane navigation. Existing session setup, history failure/retry, specialist reviews, Stop, proposal review, explicit Send/Run, checkpoints and context removal remain functional.

## Verified evidence

| Suite | Expected | Passed |
| --- | ---: | ---: |
| Workspace, floating-panel lifecycle and fill-only starters: desktop/mobile | 48 | 48 |
| Chat missing session, history/retry, scroll, proposal, specialists and cancellation | 16 | 16 |
| Canvas selection, drafts, resizing, graph/context actions and chat submission | 26 | 26 |
| Creation dialogs/branding: 1440px, 390px and 320px | 15 | 15 |
| Total browser checks | 105 | 105 |
| Automated WCAG A/AA scans: welcome, canvas and stages at two widths | 6 | 6 |

Zero browser page errors and zero automated accessibility violations. All cases have IDs in [the results file](2026-11-16-floating-assistant-results.json); remaining IDs: none. New lifecycle checks failed against the previous version, then passed after implementation. Existing tests were updated only for the intentional welcome heading change.

Svelte check reports zero errors/warnings. Docker production frontend build passed. Independent read-only code review found no important regressions. Desktop and mobile captures were visually inspected; native shared-browser inspection confirmed the deployed panel geometry and removal of the redundant tab bar. Local HTTP and Tailscale HTTPS returned 200. The frontend alone was deployed; backend data and authentication volumes were preserved.

Browser API writes and model calls were intercepted fixtures. No paid generation or live native inference was submitted. Real iOS/Safari, assistive technology and live model quality remain unverified. This is scoped implementation verification, not a repeat of the complete application checklist audit. No new design detector suppressions were added.
