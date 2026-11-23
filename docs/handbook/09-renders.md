# 9. Render jobs and outputs

[Handbook](README.md) · Previous: [Studio](08-studio.md) · Next: [Troubleshooting](11-troubleshooting.md)

Open **My videos** to monitor scene jobs, compare outputs, and download media. A job is an attempt; an output is a local result from a successful attempt.

## Understand completion

| State or stage | Meaning |
|---|---|
| Create operation succeeded | Planning and submission finished; video jobs may still be running |
| Pending/running | The provider request is queued or generating |
| Downloading | VideoFlow is retrieving the provider output |
| QA | The app is attempting a visual review |
| Succeeded | Output handling finished and QA was attempted |
| Failed | Read the error to identify submission, polling, or download failure |

Succeeded does not guarantee that QA passed. QA is best effort and can be absent when its model, FFmpeg, or frame extraction is unavailable.

## Review a take

1. Choose **Latest** to see scene groups with recent render activity first, or **Story** to return to the story's scene order. Select a scene group in My videos.
2. Play the output and inspect identity, action, visual artifacts, timing, and speech.
3. Review available QA issues and score.
4. Select the take you want to keep where offered. Selection is scoped to a job's outputs, not a project-wide final edit.
5. Download the **original** video, or the captioned variant after generating captions.

## Self-check

After the app makes a character picture, a scene picture (storyboard) or a scene clip, it can ask your own local AI session (the Codex or Claude Code session the chat uses) to look at it and say whether it is good enough. The result shows as a **Self-check** badge on the picture or clip (Library preview, the scene's references, My videos, and chat messages) with its findings behind **Details**:

- **Checked** (pass), **Needs a fix**, or **Check failed**, with a score from 0 to 100, what is wrong, and a suggested prompt change.
- **Check again** asks again even when nothing changed; **Keep it** keeps a flagged picture or clip so nothing stops on it.

What it looks at: character pictures against the character's notes (face close-up and full body both there, one character, no text or watermarks, no distorted anatomy, not a real person's likeness); scene pictures against the scene, its shots and the cast's pictures (and whether the shape matches the scene's format, which is measured, not guessed); clips through three frames (first, middle, last) against the scene, its shots and its cast.

What it cannot see: **sound**. Voices, music and lip-sync are never judged, because still frames carry no sound; every clip result says so. It also cannot see motion between the sampled frames.

When it runs:

- **Manual and Auto:** only when you press **Check** or **Check again**. Findings are shown; nothing else happens.
- **Full auto:** before any video is paid for, the pictures the videos are made from are checked. A flagged picture worth retrying is made again **once** with the suggested change (image credits) and checked again; if it is still flagged, the run stops and shows it. Clips are checked once they are made and are **never** re-rendered by themselves: a flagged clip stops the run before joining, and the assistant asks “Re-render with this change” (video credits) or “Keep it”.

The stage map's **Review** stage shows the clip findings: it is blocked while a flagged clip has not been kept or made again, and done when every clip passed or was kept.

Cost: one turn on your local session per check, and no credits. An unchanged picture or clip is not checked twice (the result is stored with a hash of the file and what it was judged against), and a project runs at most 40 checks an hour. If the session is busy, signed out or gives an answer that does not fit, the result is “Check unavailable”, which never holds anything back.

## Retry deliberately

A failed-job **resubmit** reconstructs the saved render spec and sends another provider request. A corrective output retry adds QA feedback. Either can create a new charge. A download failure can happen after the provider already generated and billed a video; inspect that job before paying for another generation.

Saved `RenderSpec` rows are mapped through the selected provider adapter. AtlasCloud H3 Developer Reference-to-Video (`minimax/h3-developer/reference-to-video`) receives image references through `refers` and a chronological prompt with spoken-dialogue instructions; resubmission preserves the stored model and prompt, then rebuilds the provider payload. A job created with H3 Developer Text-to-Video continues through that model without image references. The older `minimax/h3/reference-to-video` model ID remains supported with its own 768P/2K resolution set.

## What is currently missing

There is no complete whole-project movie assembly/export flow. Render outputs are per scene or shot. There is no end-to-end cancellation/resume contract for every orchestration stage. Restart reconciliation can re-enqueue video polling, while interrupted non-render operations are marked failed.

For OpenRouter, the worker retrieves video through the authenticated content endpoint. The contract is covered by mocked tests, but a live generation and download has not been run. A provider may complete and bill a job even if later local processing fails; inspect the job before resubmitting.
