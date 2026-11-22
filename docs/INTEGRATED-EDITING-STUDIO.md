# Integrated editing studio

Open **Studio → Edit video**, or use an existing **Trim & arrange** / **Assemble** handoff. These entry points share one workspace. Handoffs can specify an output source, scene, full story or saved revision. Unsaved edits are recovered per project and handoff; initialization must never replace the recovered draft with a fresh scene. Original media is unchanged.

## Editing and local AI

The preview, playhead, draggable visual/audio tracks, inspector, finishing controls and main ChatPanel work on the same manifest. Dragging snaps to nearby edges and frames. Arrow keys nudge by a frame, Shift by a second. Linked approved captions must be detached before changing their source clip timing. Mobile has Editor/Assistant views of the same draft.

The selected Codex/Claude session proposes edits through `/director/composition-plan`. A proposal shows changed fields and before/after values; it does not modify, save or render the edit. Apply checks that the draft still matches its original base, and normal editor Undo reverses applied edits. Native turn history retains editing requests. The server validates owned sources, actual source durations, settings, title/overlay bounds and linked-caption invariants. Existing unlinked caption wording can be corrected without inventing timings; linked approved cues remain authoritative.

## Finishing

**Transcribe final audio** mixes the actual trimmed clips and separate audio tracks locally, applying placement, mute, volume and fades. It transcribes that WAV through the configured transcription API. This can consume credits. Results become editable cues, not a rewritten source video; replacing captions is explicit and undoable. A mix without audible sources fails before provider inference.

Music stays in the same workspace with source selection, upload, trim, placement, volume and fades. **Save & export video** saves a revision and invokes the local renderer. Retrying export does not repeat transcription or video generation. Saved revisions remain available separately.

## Layouts, branding and languages

Four reusable layouts add editable title layers while retaining media. Export captures the current format and first title. Community imports use a bounded, sanitized `{format:"videoflow-layout",version:1,layout:{...}}` JSON document; they do not execute plugins or import arbitrary paths. Project branding stores colors, a supported generic font family and an owned logo with revision checks. Applying it updates title styling and adds a bounded logo overlay. Preview and export use the same text and placement fields.

Caption localization uses the selected native session, preserving exact cue IDs and timings. Translations are editable proposals. Applying stores the original and translated caption variants in the manifest; switching versions restores their cues and source bindings. It does **not** translate spoken audio, clone a voice or produce translated lip movements. Avatar creation, collaboration, arbitrary playback speeds/keyframes/transitions, automatic speech ducking and full-video dubbing are separate capabilities.

## Verification

Backend contracts cover edited audio mixing/transcription, local translation IDs/times, brand revision/ownership checks, and safe title rendering. Pure editor tests cover snapping, keyboard movement, stale proposals, cloning and layout import bounds. `scripts/e2e_integrated_studio.mjs` covers desktop/mobile editing, undo, stale proposals, music, transcription, caption variants, brand/title/logo, community layout import/export, checked reference generation, recovery and local-export handoff, plus WCAG scans.

The opt-in `TestCompositionPublicSourceRenderIntegration` also exercises a title card through actual HyperFrames structure checks, frame snapshots and video export using synthetic local media. No paid provider inference or generation is used in verification. Model artistic quality, voice identity and generated multimodal fidelity remain unverified by mocked tests.
