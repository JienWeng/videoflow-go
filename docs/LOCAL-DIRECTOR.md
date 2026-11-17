# Local director, voices and editing

Current source routes all planning and writing through your selected local **Codex** or **Claude Code** session. OpenRouter remains the vision/transcription route. fal.ai is the default new image/video route; BytePlus direct video remains selectable. Older published v0.3.2 packages do not contain this integration.

## Set up the local engine

For native backends, install and sign in to [Codex CLI](https://developers.openai.com/codex/cli/) or [Claude Code](https://code.claude.com/docs/en/setup) on the **same host and OS account as the VideoFlow backend**. Docker builds bundle Codex 0.169.0 and Claude Code 2.1.325. Installation detection does not prove login, account entitlement, quota or successful inference. The app reports a failed native turn instead of falling back to a paid OpenRouter text call.

A backend inside Docker uses its own bundled CLIs and authentication, not the login on your computer. After rebuilding, sign in from the project folder on the backend host using one of:

```sh
docker compose exec backend codex login --device-auth
docker compose exec backend claude auth login
```

Follow the provider's official flow in your browser/terminal. Codex device login must be enabled for your account/workspace; if unavailable, use the official authentication fallback. The `videoflow-agent-auth` named volume mounts `/home/videoflow`, preserving CLI credentials, configuration and native histories across container recreation. It is separate from public media storage and the project-data volume; keep it private and never export it in a project JSON. `docker compose down -v` deletes both named volumes. No host credentials are imported automatically. Existing installations need `docker compose up --build -d` to receive the bundled tools.

Provider/media keys are configured separately in Settings. Optional executable overrides are `VIDEOFLOW_CODEX_BIN` and `VIDEOFLOW_CLAUDE_BIN`; these are server environment settings, not arbitrary browser commands. Packaged native desktop previews still require separately installed CLIs.

1. Open **Local sessions** (`/director`), choose an engine and create a named session.
2. Select the characters and reference assets, then save the selection. Full appearance, personality, visual/voice rules, sample dialogue and reference bindings are included. In Assets, open a file and save identity locks such as `kind: car`, `color: red`.
3. Use Studio chat, Create video, idea development, scene/shot planning, character bibles or refinements. These use the selected native engine. Text tasks retain the project/session captured when the request started, including background work.
4. Review/edit action cards and press Run. Create pauses after generating keyframes: approve/reject them in Assets, refresh approvals, then explicitly Render each scene. Approval alone never submits video. Native agents prepare structured proposals; they have command tools disabled and cannot execute provider calls through the app. Selected image references and locked traits are passed to keyframe generation. Changing a selected asset/character invalidates earlier chat proposals, including edits while the native agent is responding. Frozen identity/voice bindings are rechecked at the provider submission boundary.
5. Return to Local sessions to inspect recorded turns, change sessions, cancel an active turn, or acknowledge an interrupted/failed turn after reviewing it. A fresh session has no native ID until its first turn starts. Saved sessions resume the exact native ID; selecting another engine creates a different session.

The native CLI owns its conversation storage. VideoFlow records a project-scoped index, requests/results and recovery states; it does not fabricate a resumed session from browser chat history or parse private native transcript files. The private workspace is beside the database under `.videoflow-agent-workspaces/<project>/<session>/`, outside served media storage. It contains app-owned AGENTS.md, CLAUDE.md, llms.txt and pinned project-local HyperFrames skills. Global agent settings are not overwritten. HyperFrames planning receives the reviewed skill contract directly because planner tools are disabled. Native JSONL events are consumed by the backend; token-by-token chat rendering is not provided.

Interrupted turns are not replayed automatically. Review the saved native session before acknowledging and sending a new instruction. The model's final JSON and the application's operation result are separate: valid JSON alone does not prove a requested generation succeeded. Activity and saved media provide execution status.

## Voices and script captions

Open **Voices & captions** (`/voices`). Saved character voices are the default; native reference-audio video is an explicit alternative. Upload a sample, bind it to the correct character, review any cloned voice, and approve exact spoken lines with stable utterance/speaker IDs. Generated scene spoken lines can seed that approval, but scene summaries are never automatically treated as speech.

Each paid stage has a prepared preview/fingerprint and explicit Run: voice creation, speech, native video, lip sync or forced alignment. Results retain upstream receipts; refresh retrieves the existing request, while an uncertain submission needs reconciliation rather than another paid attempt. Scene renders now use the saved Video & sound plan: native audio on supported models (no saved voice IDs required), saved-voice silent visuals, or silence. Saved-voice speech is a separate explicit stage. See CONNECTED-VIDEO-WORKFLOW.md for paid-call caps, grouped clips and music/assembly. A native-reference profile is rejected by ordinary first-frame rendering with a link to its compatible voice workflow. Visible saved-voice dialogue needs lip sync; narration may be a separate audio track. The initial native reference mode supports one speaker/sample and 1–9 image references per clip. Unsupported combinations fail preflight rather than dropping references.

Approved script supplies wording. Final audio supplies timing. Alignment produces editable cues and SRT/VTT/JSON; human review is still required to check actual spoken wording and voice/lip-sync quality. Existing editor preview/burn and composition caption imports preserve cue IDs and source provenance. Script/profile/sample/final media changes invalidate dependent results; guessed evenly-spaced word timing is not used. See [the detailed voice and recovery contract](voice-and-script-captions.md).

## Local composition editing

Open **Compositions** (`/compositions`). Reuse project uploads and completed video outputs, arrange normal-speed clip/audio rows, edit start/trim/duration and canvas settings, and import approved captions from the selected output. Imported cues retain source IDs and are edited in their source caption editor; trims that cut through a cue require review. Re-import after changing bound clip timing.

Describe an edit and use **Plan with local session** to populate a reviewable draft. Planning neither saves nor renders. **Save source revision**, then lint/check, review snapshots and explicitly render/import a derived asset. Every edit invalidates the prior Run revision. Advanced JSON is optional. Source files and captions are checked for changes before export; originals remain intact.

HyperFrames is optional and pinned to 0.9.33 (Node 22+, Chrome and FFmpeg). See [composition runtime setup](../internal/composition/README.md) and `scripts/setup-hyperframes.sh`. The app does not automatically install a global skill/plugin, fetch registry blocks, generate HeyGen media, transcribe, publish, cloud-render or update the runtime. Local rendering consumes your host CPU/GPU/RAM.

OpenCut is **not integrated**. Its current repository is being rewritten and the classic editor is archived. The existing video editor and HyperFrames composition UI remain available; arbitrary HTML/GSAP effects and lossless OpenCut project round trips are not advertised.

## Verification scope

[Component/E2E coverage](audits/component-e2e-coverage.md) records tested routes, controls and gaps. Tests use fake native processes and mocked paid services with real local FFmpeg/Chrome media checks. They do not prove live account access, generated identity/voice quality, every possible browser/device state or an OpenCut round trip. No paid inference is needed to run those fixtures.


### Canvas workspace

Studio includes an AI session selector. Start a Codex/Claude session there or use More tools → AI sessions for history and recovery. Select a canvas node to edit Details or open Chat with that item's scope. Suggestions fill a draft; review and submit it yourself. Settings groups preferences under General, credentials/models under AI connections, and routing/recovery under Advanced.

## Automatic character sheets in guided creation

Create story plans the cast and shots, then uses the configured reference-image route to create sheets for cast members without reference images. Codex or Claude Code prepares the prompts; the image provider draws the sheets. Existing bindings and identities are reused. Generated candidates remain pending, and the saved operation pauses at character review. Approve each design in Library, refresh approvals, then Continue storyboarding. The same operation resumes using current approved bindings and its captured media routes, without redoing character sheets. Storyboard approval and explicit video rendering remain later gates. Missing character sheets and storyboard images are separate image calls and can incur charges. Voices are configured separately.

Manual reference regeneration from canvas or Library excludes pending/rejected images as conditioning inputs. It creates a new candidate for the selected character, retaining approved active images and all prior assets. Superseded unapproved drafts are no longer active bindings after success. This reference-candidate path does not relax ordinary scene/video approval gates.
