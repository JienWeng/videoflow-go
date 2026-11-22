# Native image virtual-path compatibility

The failed 09:44 UTC native image turn reported a successful generation with a `/generated_images/<turn>/<image>.png` path. Read-only inspection confirmed the corresponding file existed under the backend account's Codex generated_images directory. The importer expected a physical absolute path and rejected the valid virtual path before reading it.

The importer now maps only the exact `/generated_images/` namespace into its configured Codex generated-image root. Existing containment, pre-turn freshness, symlink, file-size, raster-format, dimension and decode checks still apply. Arbitrary paths are not searched or imported. Native errors still do not trigger a second image API request.

Regression checks cover virtual-path imports, old files, traversal/outside paths and symlinks. The fake-native integration now returns a virtual path and verifies asset storage without contacting an image API. Full Go suite is run for this change.

No live Codex or provider request was submitted. The previously generated file is preserved, but this change does not automatically retry the failed operation or claim that the storyboard/render was completed. A fresh explicit retry may consume native generation credits.
