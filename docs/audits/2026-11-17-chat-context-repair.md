# Chat context recovery — 2026-11-17

Root cause verified from read-only running state: the selected local session retained a character ID absent from the active project's character list. Both story planning and shot planning failed while resolving that hidden session context, before invoking a model. This was separate from the visible canvas selection. The session also had an existing interrupted-turn review gate.

Changes:
- Structured `chat_context_invalid` errors in chat and specialist context validation expose an explicit Repair chat context action.
- Project-scoped repair removes only missing/foreign saved character/asset IDs, retains valid selections, and validates retained reference bindings. Database read failures fail closed. Compare-and-swap and running-session guards prevent concurrent overwrites. References requiring deeper Library repair remain blocked with a useful error.
- Recovery restores the failed prompt for editing; it never resends, generates, replays history or acknowledges a native session.
- Existing interrupted turns produce `chat_session_review_required` with a direct Review AI session link. Repair preserves this gate.
- Shared API errors retain their structured code while preserving the existing message/status behavior for other callers.

Verification: regression reproduced the exact 409 before the fix. Full Go suite and targeted API checks pass. Tests preserve owned identities, reject running/cross-project/concurrent changes, preserve review_required, and exercise repeated repair. Svelte check: zero errors/warnings; production frontend/backend builds. Browser suite: 20 desktop/mobile chat-state checks, including explicit repair, prompt restoration, no automatic AI submission, and review link. Zero page errors. No live AI/provider generation was performed.
