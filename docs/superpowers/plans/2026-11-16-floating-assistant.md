# Reference-led floating assistant

Approved brief: user explicitly requested immediate implementation matching the provided floating-chat screenshot, preserving functionality.

- Replace the edge-to-edge slab with an inset assistant surface. Reserve canvas room while open so nodes remain clickable; closing restores the full canvas. Keep pointer/keyboard resize and expand/restore.
- Use the conversation selector as the compact panel header. Hide redundant Chat-only tab navigation; selected items retain Details/Chat tabs. Closing/reopening preserves composer, history and inspector drafts.
- Place the welcome content near the top with useful fill-only prompt cards. Use one bordered composer containing context and mode/send controls; show only supported capabilities.
- Keep native session switching, cancellation, proposal approval, checkpoints and error recovery unchanged. Preserve mobile single-pane layout and labels.
- Verify desktop/mobile close/reopen, draft preservation, resizing, contextual selection, session/error states and explicit Send/Run behavior. Run the unified workspace, chat/canvas and creation regression suites plus accessibility scans. Deploy frontend only and update PR #2.
