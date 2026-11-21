# Contextual project workflow — 2026-11-16

Main task: develop a story using its characters and media, then review speech and finish video. Remove duplicate/disconnected navigation while keeping unique working tools and saved work.

## Verified changes

- Navigation: removed the miscellaneous More tools menu. Sidebar remains Studio, Library, Videos and Settings. Session tools highlight Studio, voice tools highlight Library, composition tools highlight Videos.
- AI sessions: existing assistant session dropdown remains the management entry; removed its extra Manage link. Session screen returns to Studio. Search can recall sessions directly.
- Characters: Details opens Voice & dialogue for that character, including from canvas. Character switching retains unsaved forms and navigation retains its discard guard.
- Speaker default: independent review caught a new contextual-flow mismatch: Add spoken line still selected the first character. It now defaults to the contextual character; desktop/mobile tests select Mama and assert Mama is the new line's speaker.
- Voice stages: dialogue opens when characters/lines exist, final audio when outputs exist, history/recovery when actions exist. Each disclosure remains manually recallable. Existing prepared paid actions remain visible and require explicit Run.
- Media: storyboard/reference/identity links now open the relevant Library Media view with scene_id/asset_id preserved. Existing standalone routes remain compatible.
- Final editing: Videos links to dialogue/captions and the existing composition editor; Search also recalls final editing. No stored data, editor or backend capability was deleted.

## Evidence

`2026-11-16-contextual-workflow-results.json` records every check ID, evidence description and result. Expected/reported **149/149 checks**, **16/16 accessibility scans**, no remaining selected IDs. Workflow 20; Library 32; workspace 48; canvas 26; chat 16; sidebar 7. Production frontend Docker build passed; Svelte check found zero errors/warnings; diff whitespace check passed. Independent focused review is clean after the speaker-default fix. Native browser inspected the real empty-project voice workflow with later sections collapsed.

The sidebar harness was updated to the current Library navigation and removed tool menu. The new mobile workflow test explicitly opens Workbench before using the assistant's session selector, matching existing mobile behavior. Canvas/chat initial launches lacked CHROME_BIN; both were rerun successfully with the installed browser. No new design suppressions or dependencies.

## Unverified and subjective

All browser API writes are mocked; no paid provider/native inference was run. Live synthesis, voice quality, paid lip sync, real storage integration, Safari/iOS and screen-reader behavior remain unverified in this round. Moving tools to their task owners is a usability judgment based on the requested workflow; automated checks verify access and behavior, not user preference. This focused refinement is not a new complete application checklist audit.
