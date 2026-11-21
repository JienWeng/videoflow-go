# Automatic character reference sheets

The user wants character reference images generated automatically during local Codex story creation. Codex remains the planner; the configured reference-image provider draws sheets. Missing sheets generate for the scene cast, including existing selected characters with no references. Existing reference bindings are reused without regeneration or identity edits.

Preserve human approval: generated character sheets remain pending and the saved operation pauses at awaiting_character_review before scene images. An explicit Continue storyboarding action requires current approved/available references, claims the existing operation atomically and resumes only storyboard work. It must not recreate the story, shots or character sheets or auto-submit video. The normal storyboard review/render gate remains afterwards. Existing approved cast takes the original direct path.

Record cast, reference asset IDs, scenes, brief, frozen media routes and completed stages in operation state. Validate project ownership and current scene cast. Preserve partial work on failure; duplicate continuation must not incur repeated image requests. UI shows missing sheets as included image calls, offers character review links, refresh/continue, and a new progress checkpoint. Tests use fake native/provider fixtures only; no live-paid invocation.

Alternatives: auto-approving generated identities would bypass existing review guarantees; leaving generation manual does not meet the requested automatic pipeline. A saved approval pause reuses the existing review design and preserves user control.
