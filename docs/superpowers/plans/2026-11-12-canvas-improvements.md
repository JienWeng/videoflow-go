# Canvas usability improvements

Improve the existing canvas within PR #2; preserve its graph identity and actual project data. Keep changes reversible and avoid provider calls.

- [x] Add an on-canvas finder with type filtering, visible counts and keyboard-accessible result buttons.
- [x] Use labeled Add/Arrange/Fit actions and an undo for Arrange. Preserve dirty-edit guards when focusing from the finder or global search.
- [x] Make node kinds/selection readable and align layout size estimates with rendered cards.
- [x] Match the chosen app theme, reduce unnecessary minimap/coach overlays and respect reduced motion.
- [x] Run populated desktop/mobile browser regressions, type/build checks, inspect screenshots, review and update PR #2.

Verification: 22 browser checks, zero page errors; 21 unit tests; Svelte check zero errors/warnings; production build and fresh review passed. Orphan image spacing regression was verified RED → GREEN. No paid requests or live content edits.
