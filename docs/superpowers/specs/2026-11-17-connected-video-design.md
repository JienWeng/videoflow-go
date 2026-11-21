# Connected video production

User accepted the proposed workflow and requested a checklist and implementation of every gap. Keep the Go/Svelte stack, existing scene/reference validation, local Codex image-first policy and saved-voice tools. Do not run live paid generation while verifying.

Chosen approach: one capability-aware render plan per scene, automatic grouping of contiguous timed shots into the fewest supported clips, optional per-shot control, explicit native/saved/silent audio, and ordered handoff into the existing local composition editor. Native audio is the default when the model supports it; a silent story stays silent. Saved voices are required only in saved-voice mode and remain a separate exact-speech/lip-sync workflow. No generic substitute for a chosen saved identity.

Alternatives: unconditional per-shot generation wastes minimum-duration seconds; unconditional whole-scene generation fails beyond model limits. Use greedy consecutive grouping within supported clip limits, disclose generated vs edited duration and never submit an undisclosed multi-call batch. Oversized individual shots must be replanned, not silently split into fabricated actions.

A read-only plan exposes paid call count, reused units, generated duration, source revision, audio/music choices and storyboard work. Render requests revalidate a confirmed plan ID, freeze every unit before any queue submission and insert all new jobs transactionally. Failed units are retryable only on another explicit action; uncertain units block spending. Matching outputs survive retries and page changes. Preserve plan metadata through the worker and strip it at the provider boundary.

Music: native music direction with native audio, or an uploaded/existing continuous soundtrack in compositions with gain, fades and a quiet background-level preset. Do not invent speech timings or promise exact native voices/cuts. Final captions use actual approved timing. Assembly selects one output per planned unit in order and trims generation padding; it must not concatenate alternate full-scene versions. Preserve aspect ratio and clip audio. Never generate missing media on page read.

No new provider dependencies. Official reference: https://fal.ai/models/bytedance/seedance-2.0/fast/image-to-video/api and its model overview. Native audio is included at the same generation price; no fabricated currency estimates.
