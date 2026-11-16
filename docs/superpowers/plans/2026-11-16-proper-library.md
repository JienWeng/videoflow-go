# Dedicated character and media library

User explicitly requests a proper library page rather than the current Library window. Keep the existing visual identity and data/actions; replace the Library popup/store with ordinary /library navigation. Maintain legacy /characters, /assets and ?tab=assets links.

Library has Characters and Media sections with actual collection counts. Character browsing uses visual cards, search, reference availability and sorting; preserve existing create/edit/reference-photo workflows and drafts. Media uses consistent previews, media-kind filtering, search, sorting and grid/list views, with clear empty/error states. Per-item previews and create/upload dialogs remain focused tasks; the collection itself is a full page. Keep style configuration secondary and preserve reference approval, identity locks and existing media actions.

Implementation: reuse CharacterLibrary and AssetLibrary components within a full-page LibraryWorkspace; add tested catalog helpers and a shared preview component; remove LibraryDialog and its store. Sidebar and command palette route to the library. Protect unsent chat before navigation so replacing the overlay cannot silently discard it.

Verify: canonical/legacy navigation, tab state and draft retention, search/filter/sort, images/video/audio/other previews, missing media, upload/create errors and recovery, reference approval and identity fields, navigation guards, keyboard access and mobile widths. Run focused library tests plus affected workspace, chat/canvas and creation regressions, independent review and production-image accessibility scans before frontend-only deployment and PR update. No real provider calls or native inference.
