# Connected video workflow

1. Create the story and its character/asset references. Local Codex image generation stays first where available; missing/unsupported native image generation follows the existing explicitly configured fallback.
2. Open **Video & sound** in the chat render card or Scene Studio's Video section. Choose native audio (default on audio-capable models), saved character voices, or silence. Saving options does not generate media.
3. Keep **Automatic** for consecutive multi-shot clips. The planner minimizes paid video calls, then generated seconds among equal-call plans. For a 3+2+3-second scene, Seedance Fast uses one eight-second clip. Separate-shot mode would generate three four-second clips, then trim to the eight-second edit. An individual shot beyond the model's verified limit must be replanned first.
4. Read the paid-call summary. It shows new calls, generated seconds, reused clips and possible storyboard image work. A multi-call render must carry the exact displayed plan ID. A changed scene/model/options plan is rejected before preparation. Nothing generates when opening a page, rechecking or saving options.
5. Render the plan explicitly. The server freezes all new units and inserts them in one transaction before queueing them. Completed/running matching units are reused, failed units need another explicit render action, and uncertain provider outcomes block another spend. The checkbox for another paid version applies only to completed clips and resets when that action starts.
6. Open **Assemble clips & add music**. Matching outputs load once per planned clip, in order, with generated padding trimmed and clip audio retained. Old full-scene versions are alternatives: the legacy handoff loads one, not all of them. Incomplete current plans do not silently turn into a partial film. **Assemble full story** joins that story's scenes in order; scene formats must match.
7. Add an existing soundtrack or upload audio in **Music & audio tracks**. Separate audio starts at 15% volume, with short fades. Adjust start, source trim, play length, volume (%) and fades; **Set quiet background level** restores 15%. This is a gain preset, not automatic speech detection/sidechain ducking. Use a track long enough for the film or arrange additional audio clips manually; no synthetic looping is assumed.
8. Save the edit, check playback/frames and use **Render and add to assets** for local export. The preview and exported source both carry the same gain/fade values. Imported sources remain untouched.

## Audio choices

- **Native:** synchronized dialogue, ambience and effects from the selected video model. Music direction is optional; blank requests no background music. The native Seedance route does not need Fish/ElevenLabs voice IDs. Native exact voice identity, spoken wording and cut timing are best effort; they are not approved saved-voice guarantees. fal documents native audio at the same video generation price for the configured Seedance Fast endpoint.
- **Saved character voices:** the video remains silent and the existing explicit speech/lip-sync tools use saved profiles. Saved identities and native-reference samples are never silently replaced with generic voices. A source video is needed only for the later lip-sync stage, not before rendering visuals.
- **Silent:** generate no audio. Music/narration can still be added in the editor.

For multiple native-audio clips, assign each saved spoken line to a clip when prompted. A line without an ID uses its visible stable line index. This prevents the whole scene's dialogue being repeated in every clip. It does not fabricate word-level timings. Captions require the actual final audio and verified timing, rather than evenly dividing a script.

Native music generated independently in several clips can change between clips. For a continuous film soundtrack, leave native music blank and add one music track in the editor. Audio for existing silent outputs cannot be enabled retroactively without a new generation or a separate audio edit.

## API contracts

- `GET /scenes/{id}/production`: read-only current plan, capabilities, units, new call/second totals and reuse state.
- `PUT /scenes/{id}/production`: save `settings` with the exact `updated_at` from the current plan; stale updates return 409. Audio/render settings do not invalidate visual storyboards.
- `POST /scenes/{id}/render`: `prepare_storyboard: true`, `confirmed_plan_id`, displayed `confirmed_calls`, `confirmed_seconds`, `confirmed_storyboard_calls`, and optional explicit `new_version`. Call/second/storyboard caps may decrease through reuse but cannot increase without a recheck. All scene-scoped render entry points share the scene lock and duplication fence.
- `GET /scenes/{id}/composition-draft` and `GET /scripts/{id}/composition-draft`: unsaved local edit only; no generation/export on read.
- Composition clips/audio accept optional `volume` (0–1), `fade_in` and `fade_out` (seconds; nonnegative and sum must fit the clip). Defaults preserve old saved manifests.

Official capability references: https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video/api and https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video . Capabilities vary by selected model/provider; no arbitrary “latest” model or currency price is invented.
