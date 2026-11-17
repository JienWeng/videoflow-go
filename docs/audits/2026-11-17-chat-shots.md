# Chat shot generation regression

The saved production native responses contained valid visual prompts and durations but omitted shot_order on some/all shots. The agent validator treated absent numbering as an incomplete shot and rejected the entire plan with a misleading combined error. One recorded response had durations 2, 1.2, 1.8; another had 2, 1, 2 and only the third shot numbered.

GenerateShots now uses array playback order and assigns canonical 1-based numbers. It continues to normalize numeric durations to positive integer seconds totaling the exact scene duration. The model prompt explicitly requests ordered shots and nonempty visual prompts. Invalid prompts/timings remain errors, with a shot-specific recovery message. No extra model repair/retry call is introduced. Failed background actions now display the error in the chat message instead of only inside a collapsed activity row. Existing action buttons remain available for explicit retry.

Verification:
- Regression first failed with the exact reported error, then passed for both recorded response shapes; each uses one model call and retains five seconds total.
- Full Go suite passed, including background API persistence, failed-response atomicity (existing shots retained) and explicit retry recovery.
- Svelte check: 0 errors/warnings; backend/frontend production builds passed.
- Six desktop/mobile chat checks and two axe scans passed without browser errors. Intent and first transport failure are deterministic mocks; the successful retry uses the real isolated Go router, SSE, fake local native CLI output with missing shot_order, and database persistence. No image/video calls occur.
- No live native/account or paid provider generation was run. Previously failed operations are not silently replayed; use Generate shots to retry after deployment.
