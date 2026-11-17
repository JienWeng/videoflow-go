# Automatic image workflow

User requested removal of the mandatory review flow. Scope: image consistency approval gates, progress stages and calls to approve references across story creation, chat, characters, Library and scene studios.

- Treat generated images as ready, including legacy pending images; retain explicit exclusions and project/file/revision validation.
- Generate character/prop/location references and storyboards in one run. Stop before video submission; video remains an explicit action with charges disclosed.
- Keep prior paused stories resumable without approval. Do not automatically restart old operations or spend credits on page load.
- Remove approval buttons and review-step messaging; retain optional exclusion in Details/image options, generation, regeneration, full-image viewing and version history.
- Preserve native-account recovery, interrupted-session checks, voice identity consent and provider submission reconciliation; these are separate from image review.
- Verify automatic story pipeline, legacy continuation, reuse, missing/rejected/stale inputs, regeneration and no automatic video calls using isolated fake providers; desktop/mobile browser and accessibility checks; production build and deployment through PR #2.
