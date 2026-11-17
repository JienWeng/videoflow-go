# Canvas connections

## Changes
- Explicit, keyboard-operable Connections inspector for every node: browse related items, add cast/shot assets/scene props and locations, and disconnect editable relationships. Automatic character references, scene membership, render jobs and outputs remain protected.
- Dragging uses the same compatibility policy as the inspector. Generated render/output nodes cannot initiate new links. Scene reference writes preserve the existing revision, ownership, exclusion and image-availability checks.
- Current usage supersedes overlapping generated provenance for a scene/asset pair. Duplicate edges are removed, and frontend edge IDs include relationship type. Edges leave the right handle and enter the left handle to follow the layout. Shot labels use the saved one-based order. Render nodes retain their parent scene/shot IDs for editing destinations.
- Chat receives the project graph including shots, relationships and render status. Its instructions explain connection editing and unsupported actions rather than claiming mutations. Existing project ownership and selected-node action guards remain.

## Verification
- Full Go suite passed, plus new database graph and chat-context regressions.
- Connection policy tests cover either drag direction, self/incompatible links and protected generated relationships.
- Svelte check: zero errors or warnings.
- Browser tests use mocked API responses; they do not submit live AI/provider requests. Desktop/mobile interaction checks and focused accessibility scans passed with no browser errors or violations. Results are saved beside this report.

## Limits
AI context and supported action guidance are verified; arbitrary model responses and every possible request cannot be guaranteed. Connection mutations still require explicit user interaction. No new unrestricted AI tools were introduced.

## Design triage
Preserved the incumbent semantic node-kind colors shared with the minimap/edges. Persisted a narrow EntityNode-only side-tab exception; no global/file detector exclusion was added. Typography was outside this change.
