# Browser studio: character to finished video

This guide covers the single-user browser app in this repository. A hosted server must be rebuilt/redeployed after a GitHub update; refreshing the old server does not install new code.

1. Activate a project. Studio opens on Canvas. Open the dedicated Library page for Characters and Media; search and filter collections, create profiles, or upload reference photos there. Confirm thumbnails and reference counts. Creation and item previews use dialogs. Switching collection tabs preserves editing drafts and refreshes collection data. Leaving Studio with unsent chat text requires confirming discard.
2. Use Create story from Studio, Story or Search. These entry points open the same guided dialog; /create remains its full-page view. Choose the scene count and existing cast explicitly, or allow missing character profiles to be created with the story. Start with one scene. The preflight shows the selected image/video providers and models and supported clip durations; unsupported reference/aspect combinations fail before the guided pipeline starts paid work.
3. Enter the story, language, style, aspect and direction. The effective brief and character/reference snapshot are kept with the generated scenes. Chat context and the characters appearing in a scene are separate: selecting a canvas item supplies chat context; scene cast is chosen in Story & cast.
4. Switch between Canvas and Story to review the same saved scenes. Scene stages reveal relevant tools as work progresses; expand earlier sections to review/edit them or use Show all stages to inspect later stages. Revealing a section never runs generation. View switches retain drafts; actual navigation asks before discarding them. Review scenes and shots before further generation. Regenerating scene shots replaces the previous shot snapshot atomically; it does not append duplicate shots. A concurrent edit causes the late result to be rejected instead of overwriting approved work.
Changing a scene’s cast invalidates its approved keyframes and refreshes the character/reference snapshot.
5. Generate keyframes with a reference-capable image model. A first-frame-capable video model receives the approved scene keyframe. A reference image is conditioning material, not a guarantee of exact identity or deterministic camera cuts.
6. Generate scene or shot clips. One scene render is one provider-generated clip, not a deterministic edit of individually rendered shots. Unsupported planned durations fail rather than silently truncating. Videos and Activity show progress and outputs.
7. For captions, open an output's editor: transcribe or write timed text, edit it, then Save. Save completes only after a new captioned file is produced. A failed burn keeps the previous export. The Canvas caption card burns existing saved captions; transcription model/language settings belong in the editor.
8. In Videos, select approved outputs in playback order, choose the combined aspect ratio, optionally require captioned versions for every clip, and choose Assemble selected clips. Local FFmpeg normalizes dimensions/audio, adds silent audio where needed, then concatenates without another video encode. The combined output opens in the editor and can be downloaded.

## Assistant window

Drag the assistant header or grip to undock and move it. Float assistant also opens the movable window; Dock assistant to right restores the right-side layout. Position, mode and desktop width are remembered in this browser. The move handle supports arrow keys, Shift for larger moves, and Home/End for corners. Resizing or changing the viewport keeps the window within the workspace. Mobile uses the full-width assistant pane and retains the desktop preference. Closing or switching modes keeps unsent chat and inspector drafts.

## Failure and recovery

- A browser request keeps its request key after a lost network response. Replaying it does not automatically repeat the same paid action.
- If a video submission might have been accepted but its result is unknown, the job requires reconciliation. Check the job’s saved provider account and resume the verified existing job from Videos; this does not submit another video. Do not start another generation merely because the response was lost.
- Settings shows interrupted generation responses when no current server request owns them. Review Activity, saved outputs and provider activity before explicitly unlocking; record the evidence. Unlocking starts nothing, cancels nothing and refunds nothing. A subsequent generation can incur a new charge.
- Server restart marks interrupted non-render background work as requiring reconciliation and retains saved work. It does not repeat AI stages blindly. Known provider video IDs resume polling; uncertain submission intents remain fenced.
- Re-render is an explicit new paid generation using saved inputs and requires confirmation. It is unavailable while the original job is active or uncertain. Repeated retries of an old job return its active replacement.

Local verification uses mocked provider responses plus real FFmpeg. Actual model fidelity, current model availability and the deployed server/browser still require a small, budgeted live check. The SaaS repositories and their cloud deployment are separate from this local workflow.

Provider setup, supported model profiles and legacy migration are in [PROVIDERS.md](PROVIDERS.md). Current source defaults new media routes to fal.ai, keeps OpenRouter text/vision/transcription, and offers direct BytePlus video. Existing explicit OpenRouter selections stay intact.

## Contextual tools

The sidebar contains Studio, Library, Videos and Settings. Use the story assistant’s AI session menu to manage its conversation and setup. Open a character’s Details in Library, then Voice & dialogue to work with that character; new spoken lines default to the selected character. The character selector retains drafts when switching speakers.

Voice workflow sections disclose dialogue when characters exist, final audio when rendered outputs exist, and generation history when prepared actions exist. Each section can also be opened manually. Prepared paid actions still require an explicit Run. Videos links to dialogue/captions and the final composition editor. These tools remain available through Search; existing routes and saved work are preserved. Storyboard and identity review links use the relevant Media collection in Library.

## Story creation progress

After submitting Create story, the six-step view follows saved story, scene, shot and image checkpoints, then pauses for reference review. Use Review storyboard at the top to open the first scene's references, or use the per-scene links below the steps. Approve references before choosing Render scene. The request form stays out of the way until Create another story or Edit brief & try again is chosen. Errors preserve completed steps; retry is explicit. Closing the dialog leaves the saved background operation running and Activity available. Saved legacy renders without an approval checkpoint show Review not recorded rather than an invented approval.
