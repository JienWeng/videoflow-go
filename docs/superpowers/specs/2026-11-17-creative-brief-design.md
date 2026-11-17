# Visible styling and a confirmed creative brief

Implement the user's request within the existing dark Studio identity. Make visual style prominent with named preset cards and actual preset descriptions; do not fabricate preview artwork. Reuse built-in/community presets and the saved project style. A per-story selection never silently changes the project's default.

Chat should collect and retain the idea, visual style, tone, audience, destination platform, aspect ratio, resolution, total duration, spoken language, dialogue/narration/silence and scene count. Ask for missing groups in natural language, accept free-text replies through the current AI session, and show one editable active brief rather than duplicate live cards. Carry the original story idea through follow-up answers and persist edited fields in existing conversation snapshots.

Use a shared compact editor: Look & mood → Delivery → Length & scenes → Confirm. Show all selected values in the final summary. Changing a field invalidates confirmation. The explicit final Create action starts story/reference generation; there are no image-approval stops. New version-1 UI requests must include a complete, confirmed brief and the currently displayed style fingerprint; reject missing/changed choices before provider calls. Preserve legacy API contracts and saved story snapshots.

All confirmed values must reach the saved generation brief and planning prompts. Resolution must also reach video jobs; unsupported requested resolutions fail before rendering. Shared style cards improve project style discovery and the Create story entry point. Keep safe existing defaults as labelled suggestions that the user can accept, never as hidden answers to questions.

No live paid generation in verification. Verify multi-turn context, persistence, field editing/reconfirmation, style changes, generation parameter propagation and desktop/mobile accessibility. Deliver in existing PR #2 without merging.
