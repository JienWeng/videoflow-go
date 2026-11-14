# Integrated story assistant implementation plan

**Goal:** Make canvas conversation a clear, integrated workspace using existing Svelte AI Elements.
**Architecture:** Keep the mounted inspector/chat tabs and backend contract. Use a docked panel with keyboard/pointer resizing on desktop; existing full workbench on mobile. Consolidate workflow links in a Bits UI menu, place removable context at the composer, and use a real empty state rather than a fake assistant greeting.
**Approval:** User accepted the proposed chat redesign and delegated panel/dialog choice. Docked panel is the implementation choice because canvas context stays visible; a modal would interrupt that task.
**Constraints:** Preserve dark/light tokens and existing identity, session persistence, selection guards, drafts, proposal approval. No provider/backend migration, paid calls or invented stop control.

- [x] Add browser assertions for empty state, compact navigation, keyboard resizing/expand, removable context, draft persistence and selected chat requests on desktop/mobile.
- [x] Integrate existing AI Elements message/conversation/composer with meaningful loading/error states and visible authors. Keep errors distinct from empty state.
- [x] Simplify parent panel, retain Details editor and accessible tabs; resize without remounting or losing drafts.
- [x] Run Svelte check, production build and populated canvas regression; inspect desktop/mobile screenshots.
- [x] Record checklist evidence and limits, commit and update PR.

Verification: production build and Svelte check passed; 26 populated-canvas checks, 10 chat-state checks and 14 application regression checks passed with zero browser page errors. Unit tests: 21 passed. The near-bottom scrolling regression failed before tightening the library threshold and passed afterward. Evidence and item coverage: docs/audits/2026-11-12-integrated-assistant.md.

Additional regression: completed-reply following failed when content growth was mistaken for manual history scrolling. The fix tracks movement into history separately and follows through content/composer resizing. Both ordinary following and manually paused reading are verified.
