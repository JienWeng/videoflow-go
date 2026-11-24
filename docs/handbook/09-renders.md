# 9. Render jobs and outputs

[Handbook](README.md) · Previous: [Studio](08-studio.md) · Next: [The AI editor](10-ai-editor.md)

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
- **Full auto:** before any video is paid for, the pictures the videos are made from are checked. A flagged picture worth retrying is made again **once** with the suggested change (image credits) and checked again; if it is still flagged, the run stops and shows it. Clips are checked once they are made. A clip flagged for how it *looks* is never re-rendered by itself: the run stops before joining, and the assistant asks “Re-render with this change” (video credits) or “Keep it”. A clip flagged for its *speech* gets one automatic repair first (see the speech loop below).

The stage map's **Dialogue** stage owns the speech budget before anything is made (see the speech loop below), so **Scene videos** only says which scenes have no video yet. Its **Review** stage shows the clip findings: it is blocked while a flagged clip has not been kept or made again, and done when every clip passed or was kept.

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

When it runs: in **Manual** and **Auto** only when you press **Check** on the Speech check badge. In **Full auto**, every clip made with its own sound is listened to after the frames check. The **Review** stage lists a flagged clip, for example “clip 3 (“Chase”): speech cut off at 7.9 s: line 2 stops before its end”. Without a Listening key the check says so and reads as “Check unavailable”, which never holds anything back.

### The speech loop: fit, render, listen, repair once, ask

Speech goes through one closed loop, with a hard limit on what it can spend:

1. **Write and fit.** The script writer sizes each line to the shot it is spoken in. The stage map's **Dialogue** stage (after **Shots**, before **Scene pictures**) is *ready* with **Fit script to clips** when a scene's speech won't fit its clips (free: one turn of your local AI session rewrites only what is said), *optional* when it only just fits, and *done* when every speaking scene fits. Full auto fits a scene by itself before any video is paid for.
2. **Render, then listen.** The speech check hears each clip made with its own sound.
3. **Repair.** For a clip whose line was **cut off**, said with **different words** or **missing**, the assistant can offer a card to shorten its lines and re-render it (`repair_clip_speech`: **Write shorter lines**, then **Save & re-render**). It rewrites **only that clip's lines**, shorter and simpler, from what the check heard (what was cut off or changed, when the speech ended, how long the clip is), aiming at 60–75 % of each shot's seconds instead of the usual 70–90 %. Speakers, shots, and every other clip's lines stay as they are, and no line may get longer; an answer that breaks this is refused. The lines are saved with a revision (undo them from the scene's history), then the scene's clip is made again. A scene's clips share one render plan, so the whole scene is made again.
   - **Manual and Auto:** the card shows the lines before and after and the price of the new clip (clips and seconds from the scene's render plan, plus a storyboard redraw when the changed lines make it out of date). Nothing is saved or paid for until you press **Save & re-render**.
   - **Full auto:** it runs by itself, **once per clip**: rewrite, one re-render, listen again. If the new clip is heard whole, the run carries on to joining and export.
4. **Ask.** If the clip is still flagged after that one retry, the run stops before joining and asks “Re-render again” or “Keep it”, saying it already shortened the line and re-rendered once. It never retries a second time by itself.

The one-retry limit is kept in the project's decision log, not in memory: `speech_retry:<clip id>` is written (by the assistant) just before the new clip is paid for. Any scene with such an entry for one of its clips never gets another automatic retry, after a page reload, in a second Full auto run, or from the chat card. The assistant sees the entry and can tell you “I shortened the line and re-rendered it once”.

Never retried by itself: **wrong language** and **no speech** (a shorter line can't fix those, so the assistant asks “Re-render with this change” or “Keep it”), a check that could not listen (it reads as a pass), a clip the frames check says may show a real person, a scene whose video the service refused for safety, and anything outside Full auto. In Full auto the only questions about speech are the one after the retry and the wrong-language (or no-speech) question.

**Cost, worst case.** Full auto's one confirmation states it: the story's video seconds, and if the speech check flags a clip in every scene that speaks, one more render of each of those scenes, so at most **twice the video seconds** (for example 30 s of story is at most 60 s of video). `GET /costs/estimate?full_auto=true` prices it per scene (`retry_seconds`, `worst_total`); a scene that already had its retry adds nothing. Shortening the lines is free, and listening to the new clip costs under a cent.

**Captions from script** keep the script as the source of the words and take their times from what was heard: each line shows from just before its first word to a moment after its last. A line heard close to the script (80 % alike or more) keeps the script's wording; otherwise the caption shows what was actually said, marked **Heard, not as scripted**, with **Use the script's words** to put the script back. Lines that were not heard, clips without their own sound, and edits without a Listening key keep the estimated shot timing; the note after pressing the button says how many of each. Full auto's export captions the joined video the same way.

**Voice from script** asks what to do with the clips' own voices:

- **Keep the video's own voices** (the default when the speech check heard every such clip say its lines): only clips without sound, and typed lines, get voice lines.
- **Replace with character voices**: those clips are **muted** (with their music and ambience; never just lowered, so there are never two voices at once), and each voice line starts when the character says the line in the clip. A voice line longer than its slot (until the next line, or the clip's end) is reported; **Speed up the long lines** makes it at most 1.15× faster (pitch kept) instead of overlapping. Lines that still do not fit start a little later so they never overlap. **Lower music under the voices** lowers other audio-track sound to 25 %.

Cost: listening is about $0.005–0.006 per minute of video (Google Gemini $0.005, OpenAI $0.006), so under a cent for an 8 s clip and a few cents for a whole episode; it is recorded under Costs as “Listening to a clip”, once per clip. Comparing, caption timing and the keep/replace choice are free. Voice lines cost speech credits as before; a sped-up line reuses the line already made.

## How sound works

Each scene clip is made on its own. If every clip were asked for music, every clip would make a different piece, and the music would break at each cut. So the film's sound has three parts, made in different places.

| Part | Who makes it | Where |
|---|---|---|
| Voices | The video model (Gemini Omni or Seedance), in each clip with native audio, or voice lines added in Edit video for silent clips | Each clip |
| Ambience and action sounds | The video model. Every clip of a scene gets the same ambience words, such as "light rain on a tin roof", so the room does not change at a cut | Each clip |
| Music | One track, the **music bed**, under the whole film | The edit |

**The sound plan.** Each story keeps a sound plan with its script. The plan sets the music mode (`bed`, `none`, or `clip`), the music direction, the ambience of each scene, and what the clips make. When a story has more than one clip, the clips make dialogue, ambience and action sounds only, and their prompts say there is no music. Omni prompts say "Sound design: … No music: do not add background music …". Seedance prompts use the wording from its own guide, for example "No BGM; generate only voices, environmental sounds and action sounds." A video made as a single clip can still make its own music. In **Full auto**, the AI decides the plan once, before any clip is made. A plan is kept only when it passes the checks: no music in the clips of a multi-clip story, and no music words or quoted speech in the ambience. You can also set the plan yourself (`PUT /scripts/{id}/sound-plan`). Asking the assistant for music sets the bed, and no video is made again.

**The bed in the edit.** When the scenes are joined (**Finish video**, Full auto, or opening the story in Edit video), the bed is laid under the film from the first second to the last. Its source is, in order:

1. The track in the plan: your upload or a library track.
2. A track made once from the plan's music direction with earshot. This runs on the server's CPU and costs nothing. It needs the Docker image, or a native install with `VIDEOFLOW_EARSHOT_BIN`. The track is 30 s long and loops under longer films.
3. If neither exists, the edit has no bed, and the Edit stage says "add music".

The bed plays at volume 0.35 while nobody speaks and drops to 0.14 under speech. It starts dropping 0.25 s before a line and comes back up 0.4 s after the line ends, and pauses shorter than 1.2 s stay low. Each change between the two levels is a 0.3 s crossfade. The bed fades in over 0.75 s at the start and out over 1 s at the end. Speech times are the ones the speech check heard, or the script's estimate for clips that were never listened to. Each cut between two clips' own sound gets a 0.1 s fade on both sides, so the ambience dips instead of clicking. The bed keeps playing through the cut. All of this uses the editor's ordinary volume and fade fields, so you can change any of it in Edit video.

**Room for music.** The speech fit shows, for each clip, the seconds of speech and the breathing room the clip keeps for its ambience and the music. That room is about an eighth of the clip, between 0.8 s and 2 s, for example "6.1 s of speech + 1 s for music and ambience = 7.1 of 8 s". Script writers are asked to leave that moment. It is reported, never enforced, and the speech loop's rules are unchanged.

**Where to see it.** The Edit stage line, and the assistant's answer to "what is the sound provider?", say who makes the sound. They also say whether the edit has its one bed, whether it runs without gaps, and whether it is lowered under speech. For example: "Sound: the clips' own voices, ambience and action sounds (Gemini Omni); music: one bed under the whole film from your upload “rain.mp3”, lowered under speech; in the edit: one music bed, continuous, lowered under speech 3 times; 2 cuts crossfaded".

**Costs.** Voices and ambience are part of the clip's video price. The bed costs nothing: an upload is free, and earshot runs locally. Changing the music of a multi-clip story makes no video again. Only a single-clip video that makes its own music has to be made again to change that music.

What could not be verified: no Google or BytePlus page documents audio continuity across separately generated clips, or a "no music" phrase. The wording follows each guide's general advice; see [Provider routing](../PROVIDERS.md#sound-across-clips-music-and-ambience). The mix levels were checked by the tests, not by ear.

## Retry deliberately

A failed-job **resubmit** reconstructs the saved render spec and sends another provider request. A corrective output retry adds QA feedback. Either can create a new charge. A download failure can happen after the provider already generated and billed a video; inspect that job before paying for another generation.

Saved `RenderSpec` rows are mapped through the selected provider adapter. AtlasCloud H3 Developer Reference-to-Video (`minimax/h3-developer/reference-to-video`) receives image references through `refers` and a chronological prompt with spoken-dialogue instructions; resubmission preserves the stored model and prompt, then rebuilds the provider payload. A job created with H3 Developer Text-to-Video continues through that model without image references. The older `minimax/h3/reference-to-video` model ID remains supported with its own 768P/2K resolution set.

## What is currently missing

There is no complete whole-project movie assembly/export flow. Render outputs are per scene or shot. There is no end-to-end cancellation/resume contract for every orchestration stage. Restart reconciliation can re-enqueue video polling, while interrupted non-render operations are marked failed.

For OpenRouter, the worker retrieves video through the authenticated content endpoint. The contract is covered by mocked tests, but a live generation and download has not been run. A provider may complete and bill a job even if later local processing fails; inspect the job before resubmitting.
