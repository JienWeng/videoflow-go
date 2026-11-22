# Unified node editing and manual composition plan

Use existing canonical Library, scene studio, captions editor and pinned HyperFrames adapter. Do not create competing records or pretend static reference images have video trim controls.

| Node | Existing manual tools | Work in this change |
| --- | --- | --- |
| Character | Identity, appearance, personality, visual/voice rules, reference versions/review | Direct full Library and dialogue entry points, matching node fields |
| Scene | Script, shots, cast, shared references, storyboard review, render | Direct scene studio and scene-clips composition entry |
| Shot | Prompt, integer duration, camera, motion, history | Parent scene studio link |
| Asset | Identity, name/description, versions, review, usage | Full metadata in node; shared Library; source-bound composition editing |
| Output | Captions, transcription, QA, retry, download | Direct trim/arrange composition, source captions link |
| Render job | Status/progress | Related scene/output routes; no edit of generated job internals |

Manual composition work: source-aware seeding, protected project scope, media metadata endpoint, live visual preview and seekable layered timeline; split/duplicate/reorder and undo/redo; retained source trim/start/duration controls; fit/fill, zoom, position, brightness/contrast/saturation/opacity and mute. Image adjustments are non-destructive composition edits; shared identity references remain unchanged. Saved revisions remain immutable. Linked captions stay approval-bound and require reimport after timing edits.

Validation: pure edit-operation tests, backend adjustment bounds/document escaping and source scoping tests, full Go/Svelte checks, desktop/mobile real-router browser flow, actual pinned local renderer smoke test where available. No external generation or paid provider calls. Keep unsupported features (e.g. arbitrary speed, transitions, arbitrary HTML) explicit rather than fake controls.
