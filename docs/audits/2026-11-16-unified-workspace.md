# Unified workspace verification — 2026-11-16

Studio now defaults to Canvas. Story reuses the existing scene editor against the same saved project; switching preserves mounted drafts and conversation. Project summary remains under the Project menu. Primary navigation is Studio, Library and Videos, with settings and advanced tools still accessible.

Characters/assets share a Library drawer with persistent tabs and a full-page route. Studio, Story and Search share a guarded Create story dialog backed by the existing guided creator. The separate scene script generator and repeated empty-state creation actions were removed. Legacy /characters, /assets, /create and /scenes links remain valid; full editor routes do not open duplicate modal instances.

Scene sections follow actual saved progress. Earlier sections remain expandable; Show all stages exposes later tools without running them. Saved assets can be recalled before shots exist. Filters omit stages absent from the project. Generation actions live within their stage. Canvas selection exposes contextual Details, Scene stages, Connections and Clear controls. Existing reference/voice/ownership/provider checks and revision history remain intact.

## Verified findings and fixes

| ID | Severity | Evidence/location | Concrete fix | Verification |
| --- | --- | --- | --- | --- |
| UW-R01 | High | Independent Canvas/Story drafts sent every field and could overwrite unrelated saves (`NodePanel`, `StoryWorkspace`). | PATCH only changed fields; merge refreshed values per field against the prior baseline. | Desktop/mobile browser saves Story summary then Canvas title; both survive. Four draft-helper assertions. |
| UW-R02 | Medium | Opening Library over a full library/character/asset editor duplicated IDs and could focus background tabs. | Route existing editor navigation through the full Library; use per-instance tab/panel IDs. Focus existing /create rather than duplicating it. | Duplicate-ID and keyboard-focus checks; creator palette check at both widths. |
| UW-R03 | Medium | Scene asset stages used an unhydrated generated-assets map; first-load storyboard lookup read stale scenes. | Hydrate saved associations and look up storyboard using freshly fetched scene rows. | Saved assets without shots and first-load storyboard checks at both widths. |

Fresh independent follow-up review confirmed all three corrections; no further important findings in that focused review.

## Verification and counts

All checks ran on the deployed Docker production frontend with intercepted fixture APIs. Fake writes comprised four explicit cross-view scene PATCHes, character/asset fixture creation and mocked chat/editor operations. No provider or live native inference was submitted.

| Suite | Expected | Reported passed |
| --- | ---: | ---: |
| Unified workspace: 1440px / 390px | 44 | 44 |
| Character/asset dialogs and branding: 1440px / 390px / 320px | 15 | 15 |
| Canvas editing and chat context: 1440px / 390px | 26 | 26 |
| Total browser checks | 85 | 85 |
| Scene-stage and draft-merge assertions | 16 | 16 |
| WCAG A/AA scans: canvas and stage view at two widths | 4 | 4 |

Zero browser page errors and zero automated accessibility violations. Every browser check has an ID and evidence in [the results file](2026-11-16-unified-workspace-results.json). Remaining expected IDs: none. The existing canvas suite was updated for the intentional shared creator/project menu and scoped mobile Canvas controls; its preflight fixture now supplies the real response shape.

`npm run check`: zero errors/warnings. Host and Docker frontend builds passed. Only the frontend service was replaced; backend, project data and private native authentication volumes were retained. Local HTTP and Tailscale HTTPS returned 200. A read-only native browser check confirmed Canvas default, one visible Create story button and no horizontal overflow at 1280px.

This is implementation regression verification, not a new claim that every item in the full application design guide was re-audited. Live paid model behavior, real Safari/iOS and screen-reader operation remain unverified. No subjective visual suggestions or new detector suppressions were added; the existing visual identity and scoped Inter exception were retained.
