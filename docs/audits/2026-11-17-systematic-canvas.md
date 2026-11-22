# Systematic canvas arrangement

Replaced generic graph ranking with deterministic semantic placement. Scenes follow scene_order. Each scene occupies a band with stage columns for scene, shots in shot_order, storyboard/media, render jobs and outputs. Explicit scene ownership takes precedence; graph descendants follow their scene even when linked only through a shot. Shared character hubs do not mix cast cards into the shot column.

Characters form a separate, naturally sorted band below story work, each aligned with its reference images. Shared images appear once. Remaining media uses a consistent grid. Saved manual positions remain authoritative during ordinary refresh; fresh cards avoid both pinned and newly placed cards using their full estimated dimensions. Arrange recomputes positions explicitly and the existing Undo restores the previous layout. Existing saved arrangements are not silently migrated: refresh and choose Arrange.

Viewport fitting reserves space for the toolbar and minimap, including the initial scene view and focused views. Graph records, relationships, titles, orders, media and generation state are unchanged.

Verification: five layout regressions passed (image/character heights, semantic ordering, shuffled-input determinism, manual-position collision handling and render/output descendants). Svelte check 0 errors/warnings; production frontend build. Eight desktop/mobile browser checks and two axe scans passed, with no browser errors or API writes. Covers pinned initial positions, ordered groups/reference alignment, toolbar clearance, repeatable Arrange and Undo. Rendered screenshots inspected at desktop/mobile sizes. Browser fixtures use mock data/media; no AI or provider generation.
