# Project browser and detail pages

Approved direction: dedicated Projects collection with folder cards and a semantic table; Create project in an accessible dialog; move style editing to each project's detail URL. Preserve existing activation, export/import, rename/delete and project-scoped data.

- [x] Replace inline creation/style panels with searchable, sortable folder/table views; persist view preference.
- [x] Add guarded create/rename dialogs and action menus. Prevent duplicate creation after an activation failure.
- [x] Add /projects/detail?id=… with metadata and active-project style controls. Browsing never activates a workspace.
- [x] Verify desktop/mobile, keyboard/accessibility, create failures/retry, activation scope, table/grid parity and export/import; build and update PR #2.

Use existing Svelte 5/shadcn dialogs, typography, logo and colors. No new dependency, provider calls or backend schema change.
