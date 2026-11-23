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

What it cannot see: **sound**. Voices, music and lip-sync are never judged from frames, because still frames carry no sound; every clip result says so. What is *said* is the speech check's job (below). The frames check also cannot see motion between the sampled frames.

When it runs:

- **Manual and Auto:** only when you press **Check** or **Check again**. Findings are shown; nothing else happens.
- **Full auto:** before any video is paid for, the pictures the videos are made from are checked. A flagged picture worth retrying is made again **once** with the suggested change (image credits) and checked again; if it is still flagged, the run stops and shows it. Clips are checked once they are made and are **never** re-rendered by themselves: a flagged clip stops the run before joining, and the assistant asks “Re-render with this change” (video credits) or “Keep it”.

The stage map's **Review** stage shows the clip findings: it is blocked while a flagged clip has not been kept or made again, and done when every clip passed or was kept.

Cost: one turn on your local session per check, and no credits. An unchanged picture or clip is not checked twice (the result is stored with a hash of the file and what it was judged against), and a project runs at most 40 checks an hour. If the session is busy, signed out or gives an answer that does not fit, the result is “Check unavailable”, which never holds anything back.

## Speech check and audio timing

A clip made with its own sound (for example Gemini Omni Flash with native audio) speaks whenever the video model chose, not when the shot plan estimated. The video model may also stop a line when the clip ends, say different words, or speak another language. The app handles this in three places.

**What is heard.** The finished clip's sound is transcribed once, with word times, by the **Listening** service chosen in AI setup (OpenAI `whisper-1`, or Google Gemini 3.5 Transcribe; with none chosen, whichever key is saved, OpenAI first). The listener detects the language itself, so a clip spoken in the wrong language is heard as spoken. The script goes along as a hint (OpenAI only) so names, spellings and the Chinese character set stay as written. The transcript is stored on the clip and reused by everything below, so a clip is paid for once.

**The speech check** (badge **Speech check** next to **Self-check** on My videos clips and in chat) compares what was heard with the clip's script lines:

- for each line, what was heard, how alike it is (character by character for Chinese and Japanese, word by word otherwise; traditional and simplified Chinese count as the same), when it is spoken, and whether it was *said*, *changed*, *missing* or *cut off*;
- **cut off**: the last word ends within 0.25 s of the clip's end while its line is unfinished (or later lines were never reached). Finished speech that runs into the last 0.25 s is only a note, because a trim there would clip it;
- **wrong language**, recognized by writing system (Chinese, Japanese, Korean, Latin alphabet and so on): it cannot tell English from Malay, for instance;
- **different words** (below 80 % alike; mostly different below 40 %), **missing lines**, unscripted extra speech, speech that starts late (after 1.5 s or 35 % of the clip), and a line heard more than 2 s (or 30 % of the clip) away from its shot.

A cut-off, changed or missing line makes the verdict **Needs a fix**, with a suggested prompt change (for a cut-off line: fewer words, or a longer clip). Wrong language or no speech at all is **Check failed**. Lip-sync is never checked.

When it runs: in **Manual** and **Auto** only when you press **Check** on the Speech check badge. In **Full auto**, every clip made with its own sound is listened to after the frames check; a flagged clip stops the run before joining, and the assistant asks “Re-render with a shorter line” (when speech was cut off) or “Re-render with this change” (video credits), or “Keep it”. A clip is **never** re-rendered by itself. The **Review** stage lists it, for example “clip 3 (“Chase”): speech cut off at 7.9 s: line 2 stops before its end”. Without a Listening key the check says so and reads as “Check unavailable”, which never holds anything back.

**Captions from script** keep the script as the source of the words and take their times from what was heard: each line shows from just before its first word to a moment after its last. A line heard close to the script (80 % alike or more) keeps the script's wording; otherwise the caption shows what was actually said, marked **Heard, not as scripted**, with **Use the script's words** to put the script back. Lines that were not heard, clips without their own sound, and edits without a Listening key keep the estimated shot timing; the note after pressing the button says how many of each. Full auto's export captions the joined video the same way.

**Voice from script** asks what to do with the clips' own voices:

- **Keep the video's own voices** (the default when the speech check heard every such clip say its lines): only clips without sound, and typed lines, get voice lines.
- **Replace with character voices**: those clips are **muted** (with their music and ambience; never just lowered, so there are never two voices at once), and each voice line starts when the character says the line in the clip. A voice line longer than its slot (until the next line, or the clip's end) is reported; **Speed up the long lines** makes it at most 1.15× faster (pitch kept) instead of overlapping. Lines that still do not fit start a little later so they never overlap. **Lower music under the voices** lowers other audio-track sound to 25 %.

Cost: listening is about $0.005–0.006 per minute of video (Google Gemini $0.005, OpenAI $0.006), so under a cent for an 8 s clip and a few cents for a whole episode; it is recorded under Costs as “Listening to a clip”, once per clip. Comparing, caption timing and the keep/replace choice are free. Voice lines cost speech credits as before; a sped-up line reuses the line already made.

## Retry deliberately

A failed-job **resubmit** reconstructs the saved render spec and sends another provider request. A corrective output retry adds QA feedback. Either can create a new charge. A download failure can happen after the provider already generated and billed a video; inspect that job before paying for another generation.

Saved `RenderSpec` rows are mapped through the selected provider adapter. AtlasCloud H3 Developer Reference-to-Video (`minimax/h3-developer/reference-to-video`) receives image references through `refers` and a chronological prompt with spoken-dialogue instructions; resubmission preserves the stored model and prompt, then rebuilds the provider payload. A job created with H3 Developer Text-to-Video continues through that model without image references. The older `minimax/h3/reference-to-video` model ID remains supported with its own 768P/2K resolution set.

## What is currently missing

There is no complete whole-project movie assembly/export flow. Render outputs are per scene or shot. There is no end-to-end cancellation/resume contract for every orchestration stage. Restart reconciliation can re-enqueue video polling, while interrupted non-render operations are marked failed.

For OpenRouter, the worker retrieves video through the authenticated content endpoint. The contract is covered by mocked tests, but a live generation and download has not been run. A provider may complete and bill a job even if later local processing fails; inspect the job before resubmitting.
