# Contextual project workflow

User request: integrate voice, AI sessions and Library into the workflow; remove redundant or disconnected entry points. Preserve existing saved work and working tools.

Approach: keep the existing task editors and APIs, move entry points to their owners, and hide later-stage controls behind a recallable disclosure. Removing entire editors would discard unique functions; embedding every editor in Studio would recreate the overloaded interface.

- Sidebar: Studio, Library, Videos and Settings; remove miscellaneous More tools grouping.
- Assistant: existing session menu owns session management. Remove extra Manage link.
- Character details: open Voice & dialogue scoped to that character, with an All characters selector. Switching character preserves forms.
- Videos: dialogue/captions and final editing links. Legacy routes and saved compositions remain valid.
- Voice: return to Characters, Story and Videos; final audio tools collapsed before outputs exist and manually recallable.
- Canonicalize workflow links to Library tabs; preserve legacy deep links for compatibility.

Verify selected-character navigation, draft guards, recallable stages, AI session access, editor access, responsive layout, accessibility and existing Library/chat/canvas regressions with mocked API writes. No paid inference.
