# Guided story characters

The explicit Create video request may include `create_characters: true`. Its visible default-on checkbox authorizes creating character **profiles**, not separate paid portraits or voices. Existing selected characters remain authoritative. Storyboard approval and explicit video-render gates remain intact.

1. Reproduce the inherited stale session-cast failure with a fake native engine and mocked media. Guided creation must bind its explicit cast, including an empty cast, without editing the saved session selection.
2. Extend script drafts with bounded new-character descriptors, temporary `new:key` references and scene cast. Validate all references before writes. Existing selected identities may be reused; unselected/foreign identities must not be silently adopted or overwritten.
3. Atomically save new profiles, script and scene cast/speaker bindings. Roll back all records on invalid drafts or database failures. Carry the resulting cast/appearance into later shot and image planning.
4. Add the character option to Guided creator even for empty projects. Show saved-character links at the review checkpoint. Preserve idea/settings after errors; refresh the character list on successful completion.
5. Verify fake-native full workflow, invalid/cross-project references, explicit opt-out, preservation of existing characters and transaction rollback. Run frontend checks/browser coverage and backend tests, rebuild, verify read-only live health and update PR #2. Do not replay the user's generation or submit real provider calls.

Files: `internal/agents/agents.go` (draft contract/prompt), `internal/api/story_characters.go` (validation/remapping), `generation_brief.go` and `videos.go` (request scope and persistence), `internal/db/generation.go` (transaction), `frontend/src/routes/create/+page.svelte` (explicit option/results), corresponding API/DB tests and browser fixture.
