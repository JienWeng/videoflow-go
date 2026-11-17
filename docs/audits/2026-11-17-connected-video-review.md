# Connected video workflow: verification and review

Expected checklist IDs: FLOW-01 through FLOW-10 (10). Implemented and verified IDs: all 10. Remaining implementation IDs: none.

| ID | Result / evidence |
| --- | --- |
| FLOW-01 | Native/saved/silent policy reaches the worker/provider boundary; counting provider fixtures prove native audio stays enabled and local metadata is stripped. Full Go suite passed. |
| FLOW-02 | Native mode bypasses saved speaker setup; saved mode preserves ready bindings and keeps missing profiles blocked. Desktop/mobile mode-switch and all-speaker setup tests passed. |
| FLOW-03 | Optimal consecutive grouping minimizes calls, then generated seconds. 3+2+3 is one 8s clip; separate shots generate 12s. A 7+8+1 test avoids a wasteful 15+4 grouping. Unsupported oversized shots and mismatched timelines reject. |
| FLOW-04 | Read-only plan and displayed call/second/storyboard caps are revalidated before work and after freezing; stale choices, increased spend and unexpected image work reject. Reads/options saves never generate. |
| FLOW-05 | Atomic batch insertion rolls back all jobs on failure. Repeated/partial retries reuse units; uncertain outcomes fence another spend. Frozen fingerprints survive worker enrichment. Confirmed HTTP batches are idempotent at the unit level. |
| FLOW-06 | Chat, Scene Studio and story creation share Video & sound controls; follow-up actions link to scene/story assembly, music and existing audio tools. Paid-version selection resets after the action. |
| FLOW-07 | Actual FFmpeg fixture videos verify 3+2+3 trim/positions, portrait format, audio preservation and incomplete-plan rejection. Legacy full-scene versions remain alternatives. Backend/chat/Studio completion waits for every unique planned subclip; duplicates cannot fake completion. |
| FLOW-08 | Native music direction plus uploaded/library audio, volume, fade and background-level preset. Offline HyperFrames export has an audio stream and measured gain/fades matching the edit; browser Save/export handoff preserves values. |
| FLOW-09 | 35 production-image browser checks, 8 axe scans, zero browser errors/axe violations. Real local media and read-only/counting mock APIs cover the flow; no paid generation submitted. |
| FLOW-10 | Full Go suite, targeted contract/race tests, Svelte zero errors/warnings and production builds passed. Diff reviewed for correctness, ownership, duplication, data boundaries and compatibility. Delivered through existing PR #2; backend/frontend running, local API and Tailscale HTTP 200, original three job IDs preserved. No merge. |

## Media evidence

Offline export used only deterministic test frame/tone media, networking disabled and no private application/auth volumes. The four-second export's middle mean level is -37.6 dB against source -21.1 dB (delta -16.5 dB, matching 0.15 gain); fade-in and fade-out windows measure -54.4 and -62.4 dB. Audio presence was probed and levels measured, not subjectively auditioned. This proves export controls rather than native-model voice quality.

## Review and limits

- Correctness: quote increases and stale plans reject before creating video jobs. Source validation still runs again at the provider boundary. Saved identities/rejected references/native session safety are preserved.
- Security: scene/story ownership checked; settings updates use existing revision CAS; batch inserts use a transaction. API keys/auth samples are neither copied to chat nor exposed through unit metadata. Generic render requests cannot supply trusted plan/fingerprint metadata.
- Compatibility: old composition fields default to unity gain/no fades; existing scene outputs remain available. Legacy unscoped provider requests preserve their established API behavior. Existing silent videos do not acquire sound retroactively.
- Performance: short scenes generate one clip by default; grouping avoids avoidable padding. The planner uses a bounded consecutive-shot dynamic program, and browser state cleans up existing SSE subscriptions.
- Design: existing visual identity retained, advanced clip/music options collapsed. Regex-only design scan found no additional issues in its three selected Svelte files; this limited scan is not a whole-interface clearance. Real browser axe scans cover the selected workflows.

Unverified external checks: paid Seedance output quality, exact native dialogue/voice consistency, native shot cut timing, continuity between independently generated clips and each user's provider/account access. These are model-dependent and were not asserted as verified. Native music is requested in generation; a separate editable soundtrack is recommended across clips. The 15% background preset is not speech detection or dynamic sidechain ducking. Captions need actual final audio/timing; no fabricated timing was introduced. No blanket claim of platform parity or zero bugs is made.

Deployment smoke: the existing story reports native audio, automatic grouping, one eight-second clip, ready image references/storyboard and no saved voice requirement. All live smoke requests were GET-only; no new jobs or paid generation were created. Removed the stale Scene references sentence requiring image approval.
