# Guided creative workflow

Main task: direct a complete story and its media from chat, with explicit generation approval, reviewable references, clear next steps and canvas focus. Characters need full images and their own usable page; project style belongs on Projects; the floating chat needs height control.

Implementation:
1. Remember bounded floating height, pointer and keyboard bottom-edge resizing, retain dock/mobile modes and draft state.
2. Character Library stays a uniform collection. Cards open /characters/{id}; edit details, develop character with the selected AI session, upload photos, generate/regenerate references, view the whole image and approve/reject in place.
3. Active project gets a style editor with built-in/community preset selection and custom visual fields. Explicit saves preserve project scope and drafts.
4. Chat proposals support story+reference pipeline, character references and standalone images. New story calls stop before video rendering in both text modes; existing review gates stay authoritative. Images use existing Codex-first tracked image path, not unrestricted tool execution.
5. Chat shows current prerequisites and focuses the matching canvas node while retaining chat. Completed story references offer approval and continuation; replacement reference approval uses current bindings, not rejected original candidates.
6. Codex text context includes saved project style, character appearance/personality/rules/reference IDs and owned asset metadata. Native image turns remain recorded in the selected application session. Preserve isolation and no fallback after native errors.

Verification: pure window/guidance regressions; API canonical action/context/style-revision and remote review-before-render tests; full Go/Svelte checks; production builds; isolated real-router desktop/mobile tests for character/style/chat generation/review/continuation and height-window regression suite. No live paid generation. Record exact counts and limits in the completion audit.
