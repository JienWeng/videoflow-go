<!-- Source: https://aistudio.google.com/docs/omni ("Prompt guide", "Technical details"), checked 27 November 2026. Dialogue in quotes follows Google's video prompt guides (Veo-era: https://docs.cloud.google.com/vertex-ai/generative-ai/docs/video/video-gen-prompt-guide), as Omni's own guide gives no dialogue syntax. -->
## Spoken lines (Gemini Omni)

This clip is made by Google's Gemini Omni, not Seedance. Google's guides for it say: describe sound in plain sentences ("Sound design: gentle breeze, distant bird chirps"), time a cue by the second ("After 3 seconds, a woman enters the scene.", or a timecode such as `[0-5s]`), and put exact speech in quotes after who says it (`Man: "That's no ordinary bear."`). Google says English is fully supported and other languages have not been evaluated, so state the language and keep the words exact.

- Write each line inside the shot where it is spoken, once, in the form `At 1.5s, Mei says in Chinese (warm, unhurried): "如果钱不需要靠银行来信任呢？"`. Use the time given with the line in the facts (`at`); it was worked out so every line fits its shot. For a narrator: `At 0.5s, the unseen narrator says in English: "..."`.
- One speaker at a time; never two lines at the same moment. The speaker is on screen and their lips match the words; a narrator is heard and nobody's lips move.
- Write every line exactly as given, word for word, in its own language. Never paraphrase, translate, shorten, lengthen or repeat it, and do not attach expressions or actions to single words of a line.
- Say once how each voice sounds (timbre, age, warmth, pace), in the cast or the first line of that speaker.
- Describe sound in its own sentences, separate from the picture. End with one `Sound design:` sentence for the ambient sound, and leave the last second without speech.
- If nobody should speak, say "No dialogue." Leave out sounds you do not want with short plain sentences ("No extra sound effects."); Omni has no negative-prompt field.
