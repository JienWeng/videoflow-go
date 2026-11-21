# Projects collection and detail pages

Open Projects using the project selector in the sidebar. The collection offers folder cards and a semantic table, search by name/description, and sorting by recently updated, creation date or name. The folder/table preference is remembered locally. Both views expose the same project actions through each project's menu.

**New project** opens a dialog. Name is required; description is optional. Errors preserve the entered values. Once creation returns a saved ID, a failed activation can be retried without creating that project again. Closing or navigating away from an unsaved creation/rename draft asks before discarding it. Successful creation activates the new workspace and opens its detail page.

Project names link to **/projects/detail?id=PROJECT_ID**. This page shows project metadata and counts. Browsing the URL does not activate a project. Open in Studio explicitly activates it; Activate to edit style switches the workspace before mounting its style editor. Style read/write guards continue to check project identity and saved revisions. Visual-style editing no longer occupies the Projects collection.

Import, export, rename, inactive-project deletion and command-palette activation remain supported. The active project cannot be deleted from the collection. Exports remain JSON project data and media references, not a backup of files/native sessions.

## Verification

`scripts/e2e_project_browser.mjs` intercepts all APIs and exercises 1440px and 390px layouts: folder/table parity, search/sort, view persistence, read-only detail navigation, explicit style activation, creation errors, draft cancellation, activation retry without duplicate creation, rename/export/import/delete, and failed-list retry. It checks accessibility with axe, browser errors and page overflow. Svelte diagnostics and the production frontend build are also checked. No live project mutation or provider generation is used in these tests.

The horizontal table is keyboard-focusable deliberately. A narrow Svelte compiler suppression documents that scroll-container exception; the mobile overflow regression is covered by the browser test. An initial accessibility snapshot caught the submit button mid-opacity transition; scans now wait for finite animations to settle before evaluating contrast. No accessibility rule is excluded from axe.

Verified result (2026-11-18): both viewport runs passed all 15 workflow groups each; eight axe scans reported zero violations. No browser errors or page overflow. Svelte check reported 0 errors and 0 warnings; the production frontend image built successfully.
