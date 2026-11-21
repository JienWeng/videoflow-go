# Scene speaker setup inside chat

## Problem and resulting workflow
A scene render stopped on the first missing saved speaker voice and sent the user to /voices. Historical native replies also continued to describe obsolete image approval gates and implied that source clips/final speech were needed before creating a visual clip.

The render action now checks the saved scene's shots, references and every actual speaker in one inline setup card. It shows current duration/format, preserves already-ready voices, and lets the user configure provider credentials, match missing voices, adjust individual models/IDs, and save all missing voices together. A combined **Save voices & render** action saves the chosen profiles then invokes the existing tracked render path. Images/video may consume credits; matching and configuration do not synthesize speech.

Old query-string and `speaker char_…` errors open the same setup in chat. If multiple scenes match, a scene must be selected rather than guessed. Error-to-scene association is retained in the conversation snapshot. There is no automatic replay after reload.

## Integrity and privacy
- The batch save checks current project ownership, actual scene speakers, the displayed scene timestamp and every changed voice revision. Ready voices are not overwritten. A transaction rolls back the entire batch on a conflict; incomplete batches are rejected.
- Missing speaker profiles are created only after explicit confirmation with selected saved voice IDs. No cloning, speech generation, native review acknowledgement, provider fallback or image un-exclusion is added.
- API keys use password fields and are sent only to the existing backend settings endpoint. They are cleared after saving and never placed in conversation text/options or conversation snapshots. Async matching/key writes are scoped to the displayed scene/provider; controls are disabled during saving/matching.
- The existing native session checks, reference checks, tracked render preparation, scene revision checks and provider submission fences remain.

## Assistant grounding
Each chat request includes current scene readiness, speaker statuses and explicit workflow policy. It marks usable references separately from legacy review labels. System guidance states that saved usable images do not require approval, actual saved scene settings outrank old conversation, and no pre-existing clip/final dialogue audio is required for the silent visual pass. Render follow-up text distinguishes visual rendering from later speech/lip-sync/caption timing. Historical messages are preserved rather than rewritten.

## Verification
- Full Go suite passes. New tests cover all missing speakers, preservation of existing ready voices, rejection of partial saves/foreign IDs/ready-profile replacement, transactional rollback on stale scene/profile revisions, actual speaking-character scope and language normalization.
- A real router/operation regression blocks image generation on missing voice setup, saves the voice through the new API, then successfully refreshes the storyboard and queues video using test providers.
- Browser setup workflow: nine desktop/mobile checks and two clean accessibility scans. It covers historical error recovery, inline private credentials, all-speaker matching, combined save/render, stale-profile recovery, conversation reload and no generation replay.
- Existing real-router browser render regression: six checks and two clean accessibility scans; stale storyboards update and matching images are reused.
- Svelte check: zero errors/warnings. Mechanical design scan: no findings. No live native/provider generation was submitted.

## Limits
This settles scene speaker prerequisites inside chat. It does not automatically generate final dialogue audio, lip sync or captions; those remain separate explicit paid steps. Native account review, quota/access failures, missing shots and missing/excluded references still require their applicable recovery. The card exposes their current state instead of bypassing guards or claiming that every external provider issue can be fixed automatically.
