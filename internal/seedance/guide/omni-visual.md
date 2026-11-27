<!-- Source: https://aistudio.google.com/docs/omni ("Prompt guide", "Limitations", "Technical details"), checked 27 November 2026. Veo 3.1 in the Gemini API is shut down (https://aistudio.google.com/docs/video); the Veo-era Vertex AI guide (https://docs.cloud.google.com/vertex-ai/generative-ai/docs/video/video-gen-prompt-guide) is background only. -->
## Picture and camera (Gemini Omni)

This clip is made by Google's Gemini Omni. The sections above come from the Seedance guides; where Google's Omni guide says otherwise, follow this section.

- Describe the scene, the camera movement, the lighting and the mood; be detailed about characters and the place. Specific subjects and actions avoid generic results.
- Omni cuts between several shots by default. For a clip of one shot, say "In a single continuous shot, no scene cuts." For several shots, keep the shot labels with their seconds (Google's own form is `[0-5s] A person is walking down a quiet street.`); time an event inside a shot in words ("After 3 seconds, a woman enters the scene.").
- Omni has no negative-prompt field. Write exclusions as short plain sentences in the prompt ("No dialogue.", "No extra sound effects.", "Do not add a logo."). Keep them few and put them at the end.
- Omni draws legible on-screen text when asked and when the exact words are given. Ask for text only when the facts ask for it, and then quote the words exactly; otherwise say no subtitles, captions or on-screen text.
- Refer to a reference picture by what it shows ("the person shown in the reference image"); give each character a name once and use that name afterwards.
- The clip length is set outside the prompt (3–15 s); do not write a different length into the prompt.
