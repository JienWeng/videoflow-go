# Current AI Elements chat implementation

**Goal:** Replace legacy chat primitives with the current official shadcn-svelte AI Elements compositions and add real local specialist review.
**User direction:** Implement the complete referenced AI Elements set; use local subagents when supported. Preserve existing identity and approvals. The user explicitly requested implementation without further design questions.
**Architecture:** Official Message/Message Scroller/Bubble/Input Group/Trace/Item/Marker primitives; typed bounded activity renderer for supplied real metadata; custom Go transport remains. Two application-managed, read-only local planners provide specialist review without enabling native shell/tools/recursive delegation. Unsupported model/context/terminal metadata remains truthful and conditional.

- [x] Fetch all 21 linked AI Elements markdown guides and official nova dependencies, recording provenance.
- [x] Implement safe activity normalization and conditional renderers, plus input/XSS/bounds tests.
- [x] Implement scoped capability/review endpoints with isolation, cancellation, idempotency, persistence and fake CLI tests.
- [x] Replace main messages/composer/progress/suggestions/approval chrome; connect specialist mode and saved review history.
- [x] Verify desktop/mobile, invalid capabilities, specialist failures, proposal approval, reading position, and regression suites.
- [x] Document each referenced pattern, real capability limits, evidence, and ship in PR2.

Verification: 88 browser checks, 26 unit tests, full Go suite with FFmpeg/Python and focused race tests passed. Svelte check zero diagnostics and Docker production builds passed. Independent narrative-context finding resolved and reviewed. Pattern mapping and limits: docs/audits/2026-11-12-current-ai-elements.md.
