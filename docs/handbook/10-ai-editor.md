# 10. The AI editor

[Handbook](README.md) · Previous: [Renders and outputs](09-renders.md) · Next: [Troubleshooting](11-troubleshooting.md)

Once every scene of a story has its video, the AI editor can edit the finished film for a better look and sound. It is a **separate** local Codex or Claude Code session, not your chat: it starts fresh, works in its own private folder, and hands back a proposal. VideoFlow's edit file stays the single source of truth. The AI proposes; VideoFlow checks the proposal, shows it on the timeline, and you accept it, discard it or undo it.

## Start it

- **Edit video → AI edit**, then **Start the AI edit**. It improves the edit that is open (or your current edit).
- **Chat**: ask for it ("polish the film", "add sound effects"). The assistant offers an **AI edit** card. In Manual and Auto it waits for your click.
- **Full auto**: after the clips pass their review, finishing the video runs the AI edit **once** by itself. The proposal is kept and exported only when every check below passes; otherwise the joined edit is exported as it was, and the result says why the proposal was not kept.

The editor follows the run's stage (preparing, working, checking, saving). A run can take up to 20 minutes.

## What it may change

- The order of clips and their trims, within each clip's media
- Transitions: audio fades, and visual transitions rendered as clips
- Titles and text layers, and the caption style (default, compact, large)
- Music, sound-effect and ambience tracks, and their volumes and fades
- Motion overlays (HyperFrames renders placed in an overlay box)
- Framing and colour adjustments

New pictures and sounds it makes are imported into the project as **AI edit media** (`ai_edit_media` assets), but only when the proposal is kept.

## What it may not change

VideoFlow rejects a proposal, with every reason listed, when it:

- cuts inside a spoken line, or removes or mutes a clip while it speaks (volume below 0.5);
- moves a caption off the time its words are heard (more than 0.05 s), changes a caption's words or drops it;
- uses more of a clip than its media has, plays anything at another speed, or uses media from outside the project;
- makes the film shorter than half or longer than one and a half times its length (and never over 600 s);
- changes the size or frame rate;
- puts a new full-frame clip over a speaker, or plays a new track louder than 0.5 under a line.

The proposal must also pass every check a hand-made edit passes when it is saved.

## Accept, discard, undo

A kept proposal is saved as a new version labelled **AI edit proposal**. It does not replace your edit: Edit video keeps opening your current edit, the stages keep counting it, and nothing is exported. In the AI edit panel you see what changed (cuts, order, titles, sounds, overlays, volumes, length) and choose:

- **Open in timeline** to watch and adjust it like any edit;
- **Accept** to make it your current edit;
- **Discard** to drop it. After accepting, the same button is **Undo**: the edit before it becomes current again.

The saved-edits list shows each proposal and whether it is waiting, accepted or discarded.

## Refine once by ear

After you export a proposal, **Listen to the export** runs earshot through the rendered film and keeps a second-by-second description of the soundtrack (events, sections, loudness, sync with the picture cuts) on that version. **Refine once** then gives the AI editor that description for one correction pass. The refined proposal is checked the same way. It is never refined again, and the editor never loops by itself.

Listening needs earshot's listening extra, which is off by default because its two models are large (see below). Without it, the session still makes sounds and measures their loudness.

## Cost

- **No credits.** The session uses turns of your local Codex or Claude Code sign-in, like other local planning.
- **Local CPU time.** HyperFrames renders and earshot sounds run on this computer. On a small two-core server a run can take most of its 20 minutes; the first earshot sound in a session also compiles once (about half a minute).

## Safety limits

- The session runs in its own workspace, outside public media storage, and the workspace is deleted afterwards.
- Codex runs in its workspace-write sandbox with network off; Claude Code runs Bash in its sandbox with no allowed network and no web tools.
- Its environment holds no API keys, tokens or secrets. It cannot reach your stored provider keys.
- Its tools are only `hyperframes` and `earshot`, put on its PATH as fixed wrappers. earshot runs offline and never downloads a model.
- It has a 20-minute time budget and a 1 GB disk budget; exceeding either stops it and nothing is kept. New media is limited to 40 files, 128 MB each and 256 MB together.
- Its skills (the pinned HyperFrames skills and `videoflow-editor`) are copied from VideoFlow itself. Nothing is installed while it runs.

## earshot in the Docker image

[earshot](https://github.com/Leonxlnx/earshot) (MIT) is installed from commit `7a7f7c5c70d4a763a1eca4b3666af7c77e313f97` in its own Python environment, about 500 MB. Making sounds needs no model and no network.

To add listening, build with `EARSHOT_LISTEN=1 docker compose build` (`compose.yml` passes it to the image as a build arg). This installs CPU-only torch and fetches the two listening models at build time: about 1.1 GB to download and 2.2 GB on disk. The running image never downloads anything (`EARSHOT_OFFLINE=1`). Natively, install earshot yourself, set `VIDEOFLOW_EARSHOT_BIN` to its absolute path, and set `VIDEOFLOW_EARSHOT_LISTEN=1` only once its listening models are in its cache.

## Why not Remotion or OpenCut

- **Remotion** would be a second renderer next to HyperFrames, which already renders VideoFlow's edits. Its company licence is paid above small teams, and its renders are slow on a two-core server.
- **OpenCut** is a whole editor app. It would duplicate VideoFlow's own timeline and split the source of truth.

HyperFrames (already vendored and reviewed) renders the AI's title cards and motion pieces, and earshot lets the AI make sound and hear it, while the edit file stays the one place the film is defined.

## Routes

`POST /scripts/{id}/ai-edit` (`{"revision", "refine", "auto_accept", "export"}`, `?background=true` for an op of kind `ai_edit`), `GET` and `POST /compositions/{revision}/ai-edit` (`{"decision": "accept" | "discard"}`), and `POST /compositions/{revision}/listen` (an op of kind `listen`).
