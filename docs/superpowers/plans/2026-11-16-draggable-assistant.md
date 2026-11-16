# Draggable assistant window

User explicitly requests a draggable window with a toggle between floating and right-docked modes. Keep docked as the initial default to preserve existing behavior; floating is a real overlay with no reserved canvas column. Remember the chosen mode, position and width locally. Dragging the docked header automatically undocks it. A visible move handle supports pointer dragging and arrow keys (Shift for larger moves, Home/End for corners). Clamp position on dragging, resizing and workspace/viewport changes so the header and controls remain reachable. Mobile keeps the full-width single-pane assistant and restores the desktop preference on return.

Implementation: pure tested geometry/layout parser in lib/shell/assistant-window.mjs; root Studio owns mode, coordinates and pointer capture; existing ChatPanel/NodePanel stay mounted. Retain close/reopen, resize, expand, session/history and all Send/Run/approval behavior. No backend changes or live provider calls.

Verification: failing geometry tests, mocked desktop browser drag and keyboard movement, docking, persistence, viewport bounds, pointer cancellation, close/reopen and drafts. Run current chat, canvas, workspace and creation suites; production image accessibility scans and independent review before frontend-only deployment and PR update.
