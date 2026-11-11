# Native local production sessions — implementation contract

Repository: JienWeng/videoflow-go only. Requested change: replace the Studio chat controller's stateless OpenRouter intent call with locally running Claude Code or Codex sessions that persist and resume, initialized with VideoFlow-specific guidance. Existing fal/OpenRouter/BytePlus media routing remains the execution backend.

## Prior baseline

The current chat sends the last six messages to intent_agent and retains at most 30 UI messages in browser sessionStorage. It stores no native agent session ID. Existing action cards invoke the backend endpoints separately. The local environment has Codex CLI 0.165.0 and no Claude Code executable; only CLI help/schema generation has been run, not model inference.

## Integration

- Both engines are selectable per new project session. Codex uses its documented exec JSONL interface with exact saved-ID resume; Claude Code uses its documented restricted programmatic CLI stream output and exact saved-ID resume. Do not automate terminal screenshots or parse private native session-file formats.
- Persist project ID, selected engine, native thread/session ID, workspace/context version, turn/event state and timestamps in the app database. Native engine storage remains the native session authority; the UI event log is a display/recovery index, not a fabricated native conversation.
- Resume by exact saved ID. Never use a global --last shortcut. Switching engines starts a distinct session; an explicitly transferred summary is not the same native session. Projects cannot read/continue another project's mapped session.
- Create an app-owned workspace per project under private user data. A concise AGENTS.md provides Codex instructions; CLAUDE.md imports shared guidance for Claude. A shared versioned VideoFlow workflow SKILL.md teaches project inspection, character references, scenes/shots, preflight, provider routing, durable jobs, captions and assembly. llms.txt remains the documentation index. Refresh real project state through tools, not stale transcript IDs.
- Use structured action proposals as the shared contract. Native planners have command tools disabled and do not inherit user MCP/provider configuration; the application performs inspection, validation and approved actions. This deliberately avoids experimental dynamic tools and unrestricted agent execution. HyperFrames authoring instructions are supplied directly to the composition planner as well as installed in project skill locations, so tool-disabled planners can use the reviewed contract. Runtime validation and durable jobs enforce execution independently of instructions.
- Consume bounded native JSONL event streams, save new native IDs before presenting successful session creation, serialize turns per session, support interrupt/resume, and preserve uncertain turn/tool state after process/server loss. Do not automatically replay an interrupted paid action or start a replacement native conversation while claiming it resumed.
- Use the user's installed CLI and its normal configured account. Do not copy native login tokens into the browser or overwrite global CLI config/skills. Missing installation/auth/version support produces a clear setup state. No silent fallback to OpenRouter chat.
- Native Electron and browser-launcher backends run on the host and can start installed local CLIs. A Docker backend does not automatically see host CLIs/auth; a separate host bridge is required for that deployment. Initial native integration must report that distinction explicitly.

## Confirmed execution mode

The user selected: agent plans/prepares the workflow; existing Run/confirm controls approve paid generation. Local tools may inspect project state and prepare proposals, but cannot initiate paid generation themselves. Every Run validates the exact proposal revision, selected entity/reference revisions and parameters; edits invalidate approval. No automatic paid retry loop.

## Character, asset and voice consistency (item 3)

Selected character context includes appearance, description, personality, visual/voice rules, sample dialogue and reference assets, rather than names/IDs alone. Generic assets such as vehicles/props get reusable identity profiles with locked attributes (e.g. kind=car, color=red), reference files and revisions. Planning, keyframes, video and audio use the same selected snapshots and explicit role bindings. Only the user can unlock/change an approved identity attribute.

Each editable action proposal binds its project/session, character/asset IDs, approved rules and references, speaker/voice choices, prompt, stage parameters and provider/model route. Server validation rejects cross-project/missing references, stale revisions and unsupported provider combinations before execution. The model cannot silently replace the selected red car with another entity.

Generated keyframes and sampled video frames need consistency review against approved references/attributes before reuse. Model-assisted QA can flag drift but cannot guarantee every generated frame or voice identity. Keep mismatched results for review and require a new explicit approval for regeneration. User-provided identity constraints outrank generated descriptions.

Voice samples are distinct from text voice rules. A native reference-audio mode must use an endpoint that explicitly accepts audio references and obey modality/count/duration limits. Separate saved-voice speech may provide more repeatable speaker identity; visible speech also needs a lip-sync stage and timing checks. Never claim mixing replacement audio alone synchronizes mouths. BytePlus strict first/last-frame mode cannot be mixed with omni audio-reference mode. A selected unsupported voice/reference combination must fail preflight rather than drop the voice sample.

## Confirmed text and voice scope

The user selected all text planning/writing through the chosen local Claude Code or Codex engine. This includes chat control, idea development, script/dialogue writing, scene/shot planning, asset planning, character-bible/prompt writing, and text refinements. OpenRouter remains available for vision/transcription as applicable, not as a silent fallback for failed or unavailable local text sessions. Capture the selected engine and project/session identity at the start of each background operation; switching the active project or engine cannot redirect an in-flight task. Text failures must identify the actual engine rather than attributing every error to OpenRouter.

Both voice modes are required, with saved character voices the default. Each speaker binds to an explicit character voice profile and approved sample/provider voice ID. Native reference-audio generation is an explicit alternate mode and must select a compatible endpoint; changing modes cannot silently discard voice constraints. Separately generated dialogue needs a lip-sync stage for visible speakers, while narration may be mixed without lip sync. Persist approved script/voice/audio revisions so captions and lip sync become stale when their source changes. Voice creation, speech generation, alignment and lip sync remain explicit paid actions behind Run/confirm; no automatic paid retries or fallback to an unrelated voice. A missing saved voice prompts setup or an explicit mode change instead of silently using a generic voice.

## Verification boundaries

Use fake process/protocol fixtures for both engines, saved-ID resume/restart tests, per-project isolation, malformed events/aborted turns, concurrent sends, duplicate requests, unavailable/unauthenticated engine handling, and browser session controls. Test exact installed Codex protocol compatibility without starting inference. Claude live compatibility requires the CLI to be installed; neither real model inference nor media generation is authorized for automated verification here.

## Primary references checked 2026-11-11

- Codex app-server: https://developers.openai.com/codex/app-server/
- Codex AGENTS.md: https://developers.openai.com/codex/guides/agents-md/
- Codex skills: https://developers.openai.com/codex/skills/
- Claude Code programmatic usage and session resume: https://code.claude.com/docs/en/headless
- Claude Code project guidance: https://code.claude.com/docs/en/memory

Codex dynamicTools is currently documented as experimental and requires explicit capability negotiation; native persistent thread behavior and approval handling must be version-tested. Claude Code structured output may be absent on a refused/failed run, which must not be displayed as completed work.

## HyperFrames authoring, editing handoff, and script captions

User addition: both native engines should use HeyGen HyperFrames skills, reuse imported assets and edits, optionally offer OpenCut, and caption from the approved script. This extends the implementation scope; the local rendering proof below is not app integration.

### Shared authoring workspace

Use a per-project app-owned composition workspace. Both engine workspaces discover the same reviewed HyperFrames skill source through their supported project skill locations; preserve the upstream license and provenance, pin the CLI and skill revision, and leave global agent settings unchanged. The reviewed candidate is hyperframes 0.9.33, source commit e596be16c901639ce6c18032e56924a9d8e1967d (Apache-2.0), requiring Node 22+ and local rendering dependencies. Runtime setup is an explicit optional capability, with missing prerequisites surfaced before planning a render. Existing native installers do not yet bundle this runtime.

The application workflow instructions take precedence over upstream defaults that would select HeyGen services, generate media, transcribe, publish, update skills, or call cloud rendering. Existing fal/BytePlus/OpenRouter routing and the confirmed Run gate remain authoritative. Authoring can inspect approved source assets and propose local HTML/timeline edits. A constrained app tool performs vetted local check/snapshot/render commands with argv, bounded resources and cancellation; raw unrestricted shell access is not an enforcement mechanism for paid-action confirmation.

Supplied identity assets may be used as actual image/video layers and animated with crop, position, scale or framing. That preserves their pixels; it does not make a new generated scene identity-safe. Generated scenes still require conditioning and consistency review. Sources stay in relative project asset paths; fonts are local, captions and audio have stable IDs, and timelines are deterministic and seekable. Preview source and rendered output must reference the same revision. Import rendered output as a new derived asset with source provenance rather than overwriting originals.

### Editor boundary

Preserve the existing editor while evaluating OpenCut. The current OpenCut repository is undergoing a rewrite; opencut-classic is archived. Do not represent either as a drop-in Svelte component or advertise a stable generic timeline import API without an actual bridge test. HyperFrames' own browser preview can serve composition review while that work is evaluated.

Use a versioned application editing manifest containing asset IDs/source hashes, original duration and dimensions, clip order/start/trim/speed, separate audio tracks, caption cues/styles and composition source links. The manifest is the interchange boundary, not an invented OpenCut project file. OpenCut adoption requires a pinned adapter passing real-media import, trims/reorder, audio/captions, save/reopen, export and reimport tests. Arbitrary HTML/GSAP effects cannot be promised as losslessly editable OpenCut elements; bake unsupported effects into derived clips and retain their editable source links. Unsupported features must be disclosed rather than silently discarded. OpenCut is conditional and does not block the native agent/HyperFrames path.

### Script-led captions

Store approved spoken lines with stable utterance ID, speaker/character ID, language and exact text. Scene summaries and visual descriptions are not dialogue. The approved script supplies wording; the final spoken audio supplies timing. Prefer speech-provider timestamps; otherwise offer forced alignment of final audio against the script as its own Run-approved stage. The reviewed fal endpoint is fal-ai/elevenlabs/forced-alignment, accepting audio_url and text and returning word/character timestamps. Persist its upstream task ID and recovery fence before polling; failed/interrupted alignment must not automatically resubmit a billable request.

Untimed script lines are drafts. Never infer production cue times by evenly spreading words across scene duration. Forced alignment alone does not establish that native generated speech actually matches the script; show omissions/drift for review. Keep one editable timed-cue source for player overlay, HyperFrames layers, SRT/VTT/JSON exports and existing FFmpeg burn. Invalidate alignment after script/audio replacement, and transform or regenerate cue timings after trim, reorder or speed edits. Reuse approved captions instead of letting HyperFrames init silently transcribe again; scaffold with skip-transcribe and skip-skills. No new voice, music, subscription or cloud-render service is implied by installing the authoring runtime.

### Feasibility evidence (2026-11-11)

An isolated /tmp installation of hyperframes@0.9.33 completed. A local 640x360, 24-fps, two-second imported MP4 with a timed text overlay passed lint with zero errors/warnings after declaring data-no-timeline for the static composition, and rendered a valid MP4 using system Chrome and FFmpeg. This establishes basic local media/caption rendering only: no model inference, paid provider call, native session integration, audio alignment, OpenCut round trip, packaging or application UI integration was tested. The temporary proof is not a shipped application feature.

Primary references:
- https://github.com/heygen-com/hyperframes
- https://hyperframes.heygen.com/
- https://github.com/OpenCut-app/OpenCut
- https://github.com/OpenCut-app/opencut-classic
- https://github.com/OpenCut-app/OpenCut/issues/719
- https://fal.ai/models/fal-ai/elevenlabs/forced-alignment/api

## Review resolution and final execution boundary

A fresh independent review found four Important issues, addressed in one tested fix pass: ordinary render jobs now capture director identity/voice policy and validate it again at submission; edits during a native turn reject its old-context proposal; native reference tasks bind semantic asset identity as well as file bytes; generated keyframes remain pending until explicit approval and Create stops before paid video submission. Ordinary saved-voice rendering produces a silent visual pass; approved speech/lip sync are separate explicit voice stages. Native-reference profiles require the compatible voice workflow and are rejected by ordinary I2V. Explicit retries retain the original frozen payload and must pass current source checks.

Browser-derived fixes also preserve complete chat ideas, reject text disguised as reference images, keep voice samples out of visual references, and keep long reference-review dialogs scrollable. Concurrent visual recognition updates only its owned description/tags fields; it cannot erase identity locks.
