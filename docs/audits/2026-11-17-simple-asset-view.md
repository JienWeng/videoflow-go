# Simple asset review

The asset viewer now defaults to Simple: full contained media, reference review and collapsed usage/guidance. Details reveals editing, identity locks, versions, media tools and secondary actions. Technical metadata remains collapsed. Duplicate description output was removed; the description stays editable in Details. Unsaved drafts survive mode changes and existing close protection remains active. Each newly opened asset defaults to Simple.

Fixed the stretched image container by aligning grid children at the start and bounding media height to the viewport. Preserved existing visual identity and review/write guards. No functionality removed or accessibility findings suppressed.

Verification: Svelte check 0 errors/warnings; production frontend build; six desktop/mobile browser checks and four axe scans with no violations or browser errors. Browser checks cover default disclosure state, image containment, draft retention, metadata disclosure, approval and reopening. All test API writes were intercepted; no paid generation.
