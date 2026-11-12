# Studio home dashboard implementation plan

> **For agentic workers:** Execute inline with superpowers:executing-plans and request one read-only review at the end.

**Goal:** Apply https://www.checklist.design/web-app/dashboard to Studio home, as the user explicitly selected.

**Architecture:** Add an overview to the existing Studio route. Preserve the canvas/chat workspace mounted behind a view switch, accessible directly with `/?view=studio`. Use project-scoped APIs; no fabricated metrics or paid requests. Preserve existing API defaults while allowing an explicit full operation list for recovery.

**Spec and checklist mapping:** Project name and current state orient returning users; project-total counts provide useful context; recent operations link back to work; only relevant setup/recovery blockers need attention; quick actions lead to common tasks; the empty state offers a first action. Widget customization is optional in the referenced checklist and is unnecessary for this small fixed overview. Preserve current theme and typography.

## Tasks

- [x] Write failing tests for counts, activity ordering/deduplication, actionable blockers, stale failures, first-action choice, and unknown setup status.
- [x] Implement a dashboard view-model helper and overview component using real API data, skeleton loading, error/retry and last-update context.
- [x] Add Overview / Canvas & chat switching to Studio home; preserve drafts, palette focus, and one-click direct links from AI sessions and project activation.
- [x] Update browser regression coverage for desktop/mobile empty, populated, running, recovery and API-error states; verify workspace switching and existing sessions flows.
- [x] Run Node tests, Svelte checks, production build and the dashboard/session browser checks. Visually inspect live desktop/mobile output and obtain a focused code review.
- [x] Deploy the frontend and verify the Tailscale URL.

## Constraints and review focus

- Counts must say project totals; never manufacture trends, spend or completion percentages.
- A saved session does not prove its CLI or provider credentials work.
- Dashboard actions navigate or switch views; they never submit or retry paid work.
- Unknown optional setup status is distinct from missing setup. Required data failure must not produce an empty project or zero counts.
- Shallow view switching preserves the existing mounted editing workspace; palette focus and New story handlers reveal it before interaction.
- Ruling: the previous operation API only returned the most recently created 20 items by default, so add an opt-in `limit=all` for the dashboard. This prevents older active/reconciliation work from disappearing and lets recent activity sort by update time. Existing default limits remain unchanged.
- Ruling: SvelteKit shallow routing updates `page.state`, not `page.url`; keep the view in shallow page state as well as its direct-link query. Refresh newly visible views, while pausing canvas refresh when a node form has unsaved edits.
- Only current/recent actionable issues should appear prominently. Preserve access to full activity and history elsewhere.


## Verification results

- 17 Node tests passed (11 overview rules and 6 AI-session state rules).
- 18 dashboard browser checks passed across desktop/mobile fixtures: empty/populated projects, recovery priorities, exact older-operation links, stale/error handling, unknown setup, chat draft preservation, node edit protection, refreshed totals on return, links and overflow. No paid request was submitted.
- Existing AI-session browser regression: 14 checks passed. Workspace/Settings smoke: both desktop and mobile passed. A parallel smoke run initially exceeded its 3-second Settings wait; an isolated post-deployment rerun passed.
- `go test ./...` passed with FFmpeg/FFprobe and Python available. The new API regression proved that `limit=all` includes an unresolved operation older than 20 newer records, keeps project scoping, and leaves the default 20-item response unchanged.
- Svelte check reported zero errors and zero warnings; production Docker builds passed; the design detector reported no deterministic findings.
- Native T3 browser inspection verified live desktop/mobile layout and functional Overview / Canvas & chat switching. Current project data was preserved. Screenshot: `/home/ubuntu/.t3/userdata/browser-artifacts/browser-screenshot-127-0-0-1-mwd8hk2n-34006691.png`.
- One independent read-only review found stale preserved views and truncated operations; both were corrected and covered by tests. A separate regression caught the shallow URL/state mismatch, which is also corrected.
- Relevant checklist criteria are implemented: orientation, project totals with update context, recent activity, actionable attention, quick actions and a clear first step. Optional widget customization was omitted for this small fixed overview.
- AI/provider access remains unverified on this instance because required setup is incomplete. The dashboard reports that state; no live generation quality or account access is claimed.
