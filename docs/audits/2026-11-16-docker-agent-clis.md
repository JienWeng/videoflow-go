# Docker CLI installation fix — November 16

The running backend previously had neither Node nor the Codex/Claude executables. A CLI installed in the development host is not available inside that container. This was a missing runtime dependency, not a status-label defect; detection was truthful.

The backend image now bundles Node from the Node 24 Bookworm build stage, Codex **0.169.0** and Claude Code **2.1.325**, pinned through build arguments. The final image still runs as UID 10001. Build-time version checks validate the target architecture; generated CLI state from those checks is removed from the image. No developer-host credentials or account caches are copied into it.

Compose now mounts a separate `videoflow-agent-auth` named volume at the existing `/home/videoflow` location. The home has mode 0700 and UID/GID 10001, is writable by the backend user, and is outside served media storage. CLI credentials/configuration/native histories can persist through container recreation. The existing project-data volume is unchanged; deleting Compose volumes removes both sets of data. Keep the auth volume private.

AI setup now explains bundled Docker tools and exposes sign-in commands in a collapsed disclosure. Native backend instructions remain distinct. Updated the server's host-requirement copy, `llms.txt` and `docs/LOCAL-DIRECTOR.md` so they no longer incorrectly require a host bridge for the bundled Docker route.

Verification performed:

- Before the change, executable lookup for both CLIs failed in the live container.
- Backend and frontend image builds passed. Both versions ran in the image as the unprivileged user; Codex/Claude restricted planner options and login help parsed without initiating authentication or inference.
- An isolated temporary auth volume was written by UID 10001, mounted in a second fresh container, and verified before removing that test volume. No production account data was used in that test.
- Focused director/API engine tests passed in the Go test image. Frontend type checks passed with zero errors/warnings. Compose configuration and diff checks passed.
- All **14 desktop/mobile session-page browser checks passed**, zero captured page errors. These tests use mocked engine/account state, not real inference.
- Recreated the production services without deleting data volumes. The live `/api/director/status` response reports `installed: true` for both engines, with no missing-executable reasons. Live runtime smoke checks returned Codex 0.169.0 and Claude Code 2.1.325, and verified the actual home ownership/permissions. The Tailscale endpoint returned HTTP 200.

Evidence is under `/tmp/videoflow-cli-setup/`: `build-shipped.log`, `build-frontend.log`, `shipped-smoke.log`, `codex-flags.log`, `claude-flags.log`, `auth-volume.log`, `go-test.log`, `frontend-check.log`, `director-e2e/report.json` and `live-status.json`. An initial Codex help check found a harmless stale build-cache cleanup warning; clearing generated CLI state before image publication removed it from the shipped smoke check. One HTTP read during the service restart reset its connection; the subsequent live read succeeded.

**Installation is not sign-in.** No login flow, account-token import, paid prompt or provider generation was performed. Users still authorize their selected engine using its official flow inside the backend container. The browser-managed Connect AI dialog described in the conversation remains a proposed follow-up; this fix installs the dependencies and documents the current terminal sign-in route. Authentication, entitlement and model access are still not proven by the installation flag.
