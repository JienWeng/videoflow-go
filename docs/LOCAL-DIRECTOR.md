# Local director, voices and editing

Current source routes all planning and writing through your selected local **Codex** or **Claude Code** session. OpenRouter remains the vision/transcription route. fal.ai is the default new image/video route; BytePlus direct video remains selectable. Older published v0.3.2 packages do not contain this integration.

## Set up the local engine

Install and sign in to [Codex CLI](https://developers.openai.com/codex/cli/) or [Claude Code](https://code.claude.com/docs/en/setup) on the **same host and OS account as the VideoFlow backend**. This implementation checks Codex 0.165.0 command flags; Claude needs a version supporting `--restricted` (documented from 2.1.248). Installation detection does not prove login, account entitlement, quota or successful inference. The app reports a failed native turn instead of falling back to a paid OpenRouter text call.

A backend inside Docker cannot automatically use the CLIs/authentication on your computer. Run the native backend for this local workflow; a host bridge is not bundled. Provider/media keys are configured separately in Settings. Optional executable overrides are `VIDEOFLOW_CODEX_BIN` and `VIDEOFLOW_CLAUDE_BIN`; these are server environment settings, not arbitrary browser commands.

1. Open **Local sessions** (`/director`), choose an engine and create a named session.
2. Select the characters and reference assets, then save the selection. Full appearance, personality, visual/voice rules, sample dialogue and reference bindings are included. In Assets, open a file and save identity locks such as `kind: car`, `color: red`.
3. Use Studio chat, Create video, idea development, scene/shot planning, character bibles or refinements. These use the selected native engine. Text tasks retain the project/session captured when the request started, including background work.
4. Review/edit action cards and press Run. Create pauses after generating keyframes: approve/reject them in Assets, refresh approvals, then explicitly Render each scene. Approval alone never submits video. Native agents prepare structured proposals; they have command tools disabled and cannot execute provider calls through the app. Selected image references and locked traits are passed to keyframe generation. Changing a selected asset/character invalidates earlier chat proposals, including edits while the native agent is responding. Frozen identity/voice bindings are rechecked at the provider submission boundary.
5. Return to Local sessions to inspect recorded turns, change sessions, cancel an active turn, or acknowledge an interrupted/failed turn after reviewing it. A fresh session has no native ID until its first turn starts. Saved sessions resume the exact native ID; selecting another engine creates a different session.

The native CLI owns its conversation storage. VideoFlow records a project-scoped index, requests/results and recovery states; it does not fabricate a resumed session from browser chat history or parse private native transcript files. The private workspace is beside the database under `.videoflow-agent-workspaces/<project>/<session>/`, outside served media storage. It contains app-owned AGENTS.md, CLAUDE.md, llms.txt and pinned project-local HyperFrames skills. Global agent settings are not overwritten. HyperFrames planning receives the reviewed skill contract directly because planner tools are disabled. Native JSONL events are consumed by the backend; token-by-token chat rendering is not provided.

Interrupted turns are not replayed automatically. Review the saved native session before acknowledging and sending a new instruction. The model's final JSON and the application's operation result are separate: valid JSON alone does not prove a requested generation succeeded. Activity and saved media provide execution status.

## Voices and script captions

Open **Voices & captions** (`/voices`). Saved character voices are the default; native reference-audio video is an explicit alternative. Upload a sample, bind it to the correct character, review any cloned voice, and approve exact spoken lines with stable utterance/speaker IDs. Generated scene spoken lines can seed that approval, but scene summaries are never automatically treated as speech.

Each paid stage has a prepared preview/fingerprint and explicit Run: voice creation, speech, native video, lip sync or forced alignment. Results retain upstream receipts; refresh retrieves the existing request, while an uncertain submission needs reconciliation rather than another paid attempt. Ordinary renders are silent visual passes with captured identity/saved-voice policy; speech is a separate explicit stage. A native-reference profile is rejected by ordinary first-frame rendering with a link to its compatible voice workflow. Visible saved-voice dialogue needs lip sync; narration may be a separate audio track. The initial native reference mode supports one speaker/sample and 1–9 image references per clip. Unsupported combinations fail preflight rather than dropping references.

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
