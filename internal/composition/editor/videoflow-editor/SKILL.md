---
name: videoflow-editor
description: Edit a finished VideoFlow film for a better look and sound by proposing a new version-1 edit manifest, with title cards and motion overlays rendered by HyperFrames and sound effects and ambience made by earshot. Use in a VideoFlow AI editing workspace.
---

# VideoFlow AI editor

You are editing a film whose scene videos are finished. VideoFlow's edit file (the
version-1 manifest) is the single source of truth. You **propose** a new manifest;
VideoFlow checks it, shows it on the timeline next to the current edit, and the owner
accepts or discards it. You never save, export or publish anything yourself.

## What you are given

- `input/edit.json`: the current edit (the manifest you start from).
- `input/context.json`: the clips with their media lengths, the spoken lines with the
  times they are heard, the captions, the project's style and brief, the decision log,
  and (on a second pass) `listen`, a second-by-second description of how the rendered
  film sounds.
- `input/media/`: read-only copies of the media the edit uses, named by asset id.
- `bin/`: the vetted tools on your PATH (`hyperframes`, `earshot`), when installed.
  `input/context.json` says which are available.

## What you deliver

Write exactly these, nothing else counts:

1. `out/proposal.json`:

   ```json
   {
     "summary": "Two or three sentences: what you changed and why.",
     "manifest": { "version": 1, "...": "the whole proposed manifest" },
     "media": [
       {"file": "title-open.mp4", "kind": "title", "made_with": "hyperframes", "description": "Opening title card"},
       {"file": "rain-bed.wav", "kind": "ambience", "made_with": "earshot", "description": "Soft rain under scene 2"}
     ]
   }
   ```

2. New media files in `out/media/`: names of letters, digits, `-` and `_` (at most 64)
   with one of `.mp4 .webm .mov .png .webp .wav .mp3 .m4a .ogg .flac .opus`. A clip
   uses one with `"asset_id": "new:<file name>"` (for example `"new:title-open.mp4"`).
   At most 40 files, 128 MB each, 256 MB together.

3. Your final reply: only `{"summary": "..."}`.

## What you may change

- **Order and trims** of clips within their media: `start`, `trim`, `duration` (normal
  speed only, `speed` stays 1).
- **Transitions**: short audio `fade_in` / `fade_out` on clips and music, and visual
  transitions as rendered HyperFrames clips placed between or over scenes.
- **Titles and text layers** (`texts`): id, text, start, end, size 12-160, color
  `#rrggbb`, background `transparent` or `#rrggbb`, font `sans-serif|serif|monospace`,
  position `top|center|bottom`.
- **Caption style**: `caption_style` is `default`, `compact` or `large`.
- **Music and sound tracks** in `audio`: sound effects, ambience beds and music made
  with earshot, with `volume` 0-1 and fades.
- **Volumes** of clips and tracks.
- **Motion overlays**: HyperFrames renders placed as video clips with an `overlay` box
  (`x`, `y`, `width`, `height` in percent of the frame). Use a transparent `.webm` for
  overlays drawn over the picture.
- **Framing and colour** (`adjustments`): fit, scale, focal point, brightness,
  contrast, saturation, opacity.

## Hard rules (VideoFlow rejects the proposal otherwise)

1. **Never cut inside a spoken line.** Every spoken line in `context.json` must still
   play whole: some clip of the same media must cover the line's media time
   (`media_start` to `media_end`), unmuted, at volume 0.5 or more.
2. **Keep scripted speech timing.** A line plays at normal speed with its clip; moving
   a clip moves its lines with it, nothing else.
3. **Keep the heard caption times.** Keep every caption (same `id` and `text`). A
   caption's `start` and `end` must stay on its line: new start = clip start +
   (line `media_start` - clip trim), within 0.05 s. Moving a clip means moving its
   captions by the same amount.
4. **Never exceed media.** `trim + duration` must not pass the media's length
   (`media_seconds` in `context.json`, or the probed length of your new file).
5. **Length bounds.** The film stays between `length_bounds.min` and
   `length_bounds.max` seconds and must still contain every spoken line.
6. **Keep the format.** `width`, `height` and `fps` stay as they are.
7. **Nothing hides a speaker.** A full-frame clip (no `overlay` box) must not sit on
   top of a clip while its line is spoken; draw titles and motion as overlays or put
   them between lines.
8. **Keep sound under speech.** A music, ambience or effect track playing during a
   spoken line has `volume` 0.5 or less (about 0.15 for music under dialogue).
9. Use only the asset ids already in the edit or in `context.json` `sources`, plus your
   own `new:` files. Unique ids for every clip, caption and text layer.
10. **Keep the one music bed.** When the edit already has a continuous music track
    (the story's sound plan bed, split into segments that duck under speech), keep it
    running under the whole film: you may retime its segments to new cuts and adjust
    its levels within rule 8, but do not remove it, replace it with per-clip music, or
    leave gaps between its segments. Clips themselves carry no music in a multi-clip story.

## Tools

Everything runs on this machine's CPU with no network. Do not install packages or
skills, do not use `npx`, do not fetch anything.

- **HyperFrames** (`hyperframes`, see the hyperframes skill): write a small HTML
  composition in its own folder under `work/`, then
  `hyperframes render work/title --output out/media/title-open.mp4 --workers 1`.
  Keep renders short (a few seconds each): the machine is small.
- **earshot**: make sounds from words and listen to them.
  - `earshot make "soft rain on a window, no thunder" -o out/media/rain.wav --dur 20`
  - `earshot make "a soft whoosh for a title" -o out/media/whoosh.wav`
  - `earshot sfx --help`, `earshot kit --help`: the shipped film sound bank.
  - When `context.json` says listening is available: `earshot describe FILE`,
    `earshot match FILE "words"`, `earshot timeline FILE` to hear what you made
    before you use it. Without it, `earshot measure FILE` still gives loudness.
  - The first sound takes longer (it compiles once).

## Budget

You have about 20 minutes and a limited amount of disk. Prefer a few good changes
(an opening title, a sound bed per scene, a couple of matched effects, gentle fades)
over many risky ones. When unsure about a rule, leave that part of the edit as it is.
