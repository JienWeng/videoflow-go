# Canvas workspace and Settings redesign

Approved scope: canvas + contextual chat as the primary workspace, including reducing and standardizing scattered Settings. Repository: JienWeng/videoflow-go.

## Delivered interaction

- Studio keeps the canvas central with one Details/Chat workbench. Mobile switches between Canvas and Workbench, preserving drafts and selection instead of squeezing both onto one screen.
- The workbench shows selected-item scope, current prompt/summary, native AI session selector and workflow links. Scene/shot and character fields edit inline; asset identity traits edit inline with existing revision checks. Reference photo/review/voice management uses the existing dedicated pages.
- Contextual requests send an ID/kind. The server resolves owned records, detects changes during planning, pins compatible actions to that item and makes incompatible actions non-runnable. Suggestions fill the composer rather than submitting.
- Native session changes fence both outstanding chat responses and history restoration. Changing a session does not start inference.
- Navigation groups Studio/Videos, Library and More tools. Mobile navigation uses a labelled dialog; existing routes remain valid.
- Settings has General (scene/caption defaults, negative prompts, appearance), AI connections (credentials and media models) and Advanced (routing and recovery). The former Providers/Engines/Defaults/Agents/Appearance hash links map to the new sections. Existing save handlers and generation-test boundaries remain intact. Immediate preferences and explicit credential saves are explained separately.
- Shared font/focus treatment, current-page indicators, labelled tabs, consistent spacing and mobile input targets. Themes retained. This is an implementation improvement, not a claim of complete WCAG certification.
- Corrected character visual/voice rules payload keys in both inline and dedicated editors so they match the existing API's JSON contract.

## Review and fixes

One independent review found three Important defects, all addressed with targeted regression checks: Cancel restoring a stale pre-save baseline, wrong-kind scoped actions remaining runnable, and delayed native responses/history leaking across session switches. Also disabled character Cancel while saving and updated specification approval status. No second independent approval claim is made.

## Verification

See execution logs under /tmp/videoflow-redesign-* on the development host:

- Svelte check: zero errors/warnings; production build completed.
- Full Go race suite passed; Go vet and backend build completed.
- Browser component suite: desktop/mobile route, navigation/palette, Settings sections/theme, validation, and backend-unavailable states.
- Populated canvas checks: dirty-selection cancellation, latest saved baseline, scene save, shot editor, draft preservation, suggestion no-submit behavior, selected ID sent, inline character data, asset deep link, context-menu and drag persistence.
- Session isolation checks: delayed A response after switching to B and out-of-order native history restore.
- Workspace checks: desktop/mobile session selector, workbench tabs, Settings groups and mobile navigation drawer.
- Deterministic interface detector reported no findings during the implementation pass. Desktop/mobile screenshots inspected. Mocked media/native calls only; no paid generation.

## Limits

Existing media editing boundaries remain: rendered pixels are not freely editable, render-job nodes are status records, and compositions use timing/track forms rather than freeform HTML/GSAP editing. No physical touch device, screen-reader pass or paid output-quality evaluation. Existing downloadable release packages are not rebuilt. No additional app dependencies.
