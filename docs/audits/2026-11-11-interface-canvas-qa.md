# Interface and canvas QA follow-up

Repository: JienWeng/videoflow-go. Source baseline 997c58f plus this follow-up.

## Findings and fixes

- P2: Character canvas details showed only a name, with no path to the full editor. Added Edit character, opening that character's populated form, and a saved voice link.
- P2: Asset canvas details offered deletion but no identity editor. Added the existing asset-specific identity/review deep link.
- P2: Context menu positioning could become negative on short viewports. Clamp to an 8px inset and bound/scroll the menu height.
- P2 robustness: Timeline drag state cleared on pointer up/cancel but not lost pointer capture or window blur. Added both cleanup paths. This change was checked by static/type/build validation; synthesized interrupted-touch behavior was not exercised.

## Canvas capabilities

- Story graph: nodes move and positions persist in browser storage; scene title/summary/duration/aspect and shot prompt/camera/movement/order/duration edit in the detail panel. Supported cast and asset relationships can be added/removed. Layout positions are browser-local, not shared project records.
- Character nodes: full editing in Characters via the new direct link; voice configuration in Voices & captions.
- Asset nodes: identity/review in Assets via the new direct link. Uploaded pixels are not a raster drawing editor.
- Output nodes: open the existing video editor for caption text/timing/style and shot metadata. Rendered pixels and arbitrary video objects are not directly editable.
- Render job nodes: status records, not editable jobs.
- Composition: editable clip/audio rows, timing, order, canvas/style presets and approved captions; preview/snapshot is not a freeform drag-and-drop design canvas. Arbitrary HTML/GSAP editing and OpenCut remain outside the implemented workflow.

## Executed verification

- Existing component browser suite: 36 desktop/mobile checks passed, no page errors or unknown fixture endpoints. Covers twelve routes, palette, project dialogs, settings, empty validation and backend unavailable states.
- New populated canvas suite: desktop 1440px and mobile 390px, scene edit/save, shot editor, correct character deep link, asset link, context menu/Escape, node drag persistence. Uses mocked API, not real generation.
- Screenshots inspected: empty desktop/mobile Studio and populated mobile Studio. Existing theme and layout retained.
- `npm run check`: zero errors/warnings. Production build passed. `git diff --check` clean. Impeccable detector returned no findings for the four edited UI files.
- Browser fallback: T3 preview explicitly failed because host AppArmor blocks its sandbox; used repository's external Playwright/system Chrome setup.

## Remaining limitations

Mobile graph labels are small at fit-to-graph zoom; zoom controls and detail panels remain necessary. The dense horizontal mobile navigation and mixed-language chat suggestions could use a dedicated UX pass. No physical touch device or screen reader/contrast audit was run, and this is not proof that every button/state combination is defect-free. No paid AI calls, live native engine authentication or media quality evaluation were performed. Earlier end-to-end media workflow evidence remains in the integration audit.
