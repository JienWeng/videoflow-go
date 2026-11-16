# Overview simplification — November 16

The project overview now leads with the next useful action, followed by compact setup notices and linked totals. Recent activity and simple shortcut links share a desktop row and stack on mobile. Removed the manual Refresh button, the noisy update timestamp and repeated full-width section separators. Project descriptions, counts, warnings, recovery links and recent timestamps remain truthful and available. Typography, branding and theme identity are retained.

The overview still refreshes on existing job events and on return from the canvas. It additionally reloads when its window gains focus or the tab becomes visible, only while the overview is active. Listeners are removed on unmount; overlapping requests retain the existing busy guard. Errors keep last-loaded data and the explicit Try again recovery control. No polling interval or backend change was added.

Verification:

- The new no-Refresh assertion failed against the original UI at desktop/mobile before implementation.
- **20 dashboard browser scenarios passed**, including automatic focus/visibility updates, stale data/error recovery, unknown setup, exact interrupted-request links, canvas/chat drafts and responsive layout. The test helper waits for the current request to finish before dispatching the next focus event; an initial mobile timeout exposed a test readiness race after retry, corrected without changing application behavior.
- **12 automated accessibility scans passed with zero violations**, light/dark and desktop/mobile, including overview scans plus the harness's adjacent populated-selection/dialog regressions. Those harnesses contributed 12 additional named browser checks; **32 total named checks passed**, zero captured page errors.
- **12 dashboard unit tests passed**. Production Docker build and `npm run check` passed with zero errors/warnings. Script syntax and diff checks passed.
- Live collaborative-browser inspection confirmed the new layout at 1280px and 320px, no horizontal overflow at 320px, and no Refresh overview control. The production frontend was rebuilt; backend data was not changed.

[Committed named-check evidence](2026-11-16-overview-simplification-results.json). Raw reports/screenshots/logs: `/tmp/videoflow-overview-simplify/confirmed`, `a11y-dark`, `a11y-light`, `unit.log`, `build.log` and `check.log`. Automated accessibility does not replace screen-reader/physical-device testing; existing provider/native-account boundaries remain unchanged. No font or detector exception was added in this change.
