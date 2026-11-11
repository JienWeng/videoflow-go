# Character voices and script-led captions

The `/voices` workspace binds a character to an approved voice revision. Saved MiniMax voices are the default. A profile may bind an existing MiniMax voice ID or create one from an uploaded audio sample; creation requires a separate prepared action and Run. Listen to the returned preview before accepting the new voice ID. The provider requires using a cloned voice with TTS within seven days to retain it. Voice samples remain character-bound audio assets and do not enter the visual reference list.

Native reference audio is an explicit alternate profile mode. Its curated route is `bytedance/seedance-2.0/fast/reference-to-video`, with one WAV/MP3 sample (at most 15 seconds and 15 MB) and 1–9 selected image references. This initial app workflow does not expose video references or multiple speakers within a native clip. It cannot combine strict first/last frames with audio reference mode. The app passes the actual sample bytes and approved dialogue to the reference-capable endpoint rather than silently dropping the voice constraint. Identity, exact speech and lip synchronization still need human review after generation.

Spoken lines have stable IDs, character IDs, language, exact wording and a visible-speaker flag. Importing scene dialogue creates an unapproved draft; scene summaries are never treated as dialogue. Save and approve the exact spoken script before preparing speech or alignment. A saved-voice speech result becomes a local character-bound audio asset with its script, utterance and voice provenance. Visible speakers need the explicit lip-sync stage; mixing narration into a composition does not synchronize mouths. Lip sync validates the matching utterance, script/voice revisions and audio/video durations (within 250 ms) before it can be submitted.

Final captions align the approved script to audio extracted from the selected final rendered output. The forced-alignment request contains that audio and exact text. Provider word timestamps supply cue boundaries; no timing is inferred by evenly distributing words. Every cue retains the utterance ID, character and language while using the approved wording. Alignment words/loss remain available in the stored result. A warning and manual listening/review step precede applying the cues: forced alignment alone cannot prove that generated speech actually follows the script. A mismatched or invalid word sequence is retained for review without overwriting captions.

Applying captions writes the existing output's editable `CaptionsJSON`. The player/editor, caption burn, composition import and SRT/VTT/JSON export reuse these cues. Script revisions and the final output's SHA-256 are attached as provenance. Script changes, audio/video replacement, trims, speed edits and reorder-derived output changes invalidate reuse; re-align the edited final output. Caption edits preserve the binding instead of erasing the stale fence. The existing editor can display stale cues for repair, while production burn/export must reject stale sources.

## Persistence and recovery

`voice_documents` holds project-scoped, optimistic-concurrency revisions for profiles and scripts. `voice_tasks` stores a unique preparation fingerprint, source bindings, fixed fal route, exact request payload, status, provider request ID and result. Responses expose a safe preview of wording/settings, never sample data URIs or credentials.

Preparing the same action recovers its existing task. Run requires an explicit confirmation and the exact fingerprint, revalidates source revisions/hashes, and durably changes `draft` to `submitting` before contacting fal. Only one caller can claim that transition. A transport error or missing receipt becomes `uncertain`; a server restart preserves `submitting`. Neither state is automatically resubmitted. Review fal activity and manually attach the existing request ID, then refresh to retrieve it. Result retrieval and local artifact-import recovery never submit paid generation. Upstream results are persisted before downloading artifacts; deterministic artifact IDs make local recovery idempotent.

Native reference image bindings include both file SHA-256 and a canonical semantic identity revision (name, type, description, character, tags, full metadata including locked traits, and update time). Prepared prompts carry those exact case-preserving identity constraints with their @Image bindings. Metadata-only identity changes invalidate Run even when image bytes stay unchanged.

Ordinary video requests in the local-engine workflow enforce saved voice profiles before enqueue: spoken characters need approved saved voice IDs, and the video pass requests no generated audio. Saved-voice speech and visible-speaker lip sync remain explicit `/voices` stages. Native reference profiles are rejected on ordinary image-to-video routes with a link to the compatible `/voices` workflow; they are never converted into a request that drops the audio sample. Explicit silent scenes allow a silent visual pass.

All voice endpoints use the active project and honor an optional `project_id` query fence, which the workspace sends on voice operations. Tasks from another project cannot be read, run or reconciled. Paid requests never invoke a text model. Script drafting remains the selected local Claude/Codex workflow elsewhere in the app.

## API

- `GET /voice/state`: current project's documents, task receipts, curated route names and setup limits.
- `PUT /voice/profiles/{character}`: `{revision,data:{mode,sample_asset_id,voice_id,approved}}`; default mode is `saved`.
- `PUT /voice/scripts/{id}`: `{revision,data:{approved,lines:[{id,character_id,language,text,visible}]}}`.
- `POST /voice/tasks/prepare`: `{kind,script_id,line_id,character_id,output_id,audio_asset_id,image_asset_ids,prompt,duration,aspect_ratio}`. Supply the fields for `clone`, `speech`, `native_reference`, `alignment` or `lipsync`; unsupported fields/modes fail validation.
- `POST /voice/tasks/{id}/run`: `{confirmed:true,fingerprint}`. The only route that submits a paid request.
- `POST /voice/tasks/{id}/refresh`: retrieve the saved provider request or finish local import.
- `POST /voice/tasks/{id}/reconcile`: `{reviewed:true,upstream_id}` for an uncertain submission without a saved receipt.
- `POST /voice/tasks/{id}/accept-voice`: `{reviewed:true}` after preview review.
- `POST /voice/tasks/{id}/apply-captions`: `{reviewed:true}` after final audio/cue review.
- `GET /voice/outputs/{id}/captions/export?format=srt|vtt|json`: export the same editable cue source.

## Limits and verification

FFmpeg and FFprobe on the backend host's PATH are required for sample/media validation, audio extraction and lip-sync timing checks. This app caps cloning samples at 10–300 seconds and 32 MB, alignment final outputs at 10 minutes, speech text at 5,000 characters, speech downloads at 32 MB, lip-sync input video at 50 MB and generated video downloads at 256 MB. These app limits can be narrower than provider limits. Audio extraction is local. Media probes use a bounded format/protocol allowlist; provider downloads reuse the existing SSRF-checked public-media boundary.

Tests use local mock fal HTTP endpoints and real local FFmpeg media. They cover actual audio upload and separate visual references, queue schemas/auth, approved source snapshots, project isolation, duplicate Run fencing, uncertain reconciliation, restart recovery, actual sample bytes in native mode, local speech/video import, lip-sync provenance/timing, final-audio alignment, exact wording/stable IDs, manual apply, caption exports and source invalidation. No live model inference or paid provider calls are part of these tests. Voice similarity, generated dialogue accuracy and live provider account behavior are not established by mocks.

Official schemas checked 2026-11-11:

- [MiniMax voice creation](https://fal.ai/models/fal-ai/minimax/voice-clone/api)
- [MiniMax speech HD](https://fal.ai/models/fal-ai/minimax/speech-02-hd/api)
- [Seedance 2.0 Fast reference-to-video](https://fal.ai/models/bytedance/seedance-2.0/fast/reference-to-video/api)
- [Sync lip sync Pro](https://fal.ai/models/fal-ai/sync-lipsync/v2/pro/api)
- [ElevenLabs forced alignment](https://fal.ai/models/fal-ai/elevenlabs/forced-alignment/api)
