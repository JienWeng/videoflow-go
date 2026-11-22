# VideoFlow creative preset packs

A pack is a UTF-8 JSON file named `your-name.videoflow-presets.json`.
Download the starter pack in Settings → Creative presets, edit it, then preview and install it.
Share the file directly with others; this is a local import format, not a hosted marketplace.

Required pack fields: `format` = `videoflow.preset-pack`, `schema_version` = `1`, `id`, `name`, `version`, and `presets`.
Optional fields: `author`, `license`. Attribution is supplied by the author and is not verified.
Use your own pack ID and name. ID values use lowercase ASCII letters, numbers and single hyphens, up to 64 bytes.

Each preset requires `id`, `name`, `description`, `style_prompt`, and `guidance`.
Guidance requires all four strings: `story`, `shots`, `image`, `video`.
Use story for pacing and dialogue; shots for framing and continuity; image for materials, palette and reference preservation; video for achievable movement and temporal consistency.
Do not put credentials, executable code, tool instructions, provider settings or approval overrides in a pack.
Creative guidance does not grant permissions or change provider routing.

Limits: 512 KiB per file, 1–25 presets per pack, 10 installed packs.
Name: 120 bytes; version: 32; author: 200; license: 120; description: 500; style_prompt and each guidance string: 3000.
Unknown fields, duplicate preset IDs, unsupported versions and trailing content are rejected.
Imported presets use `community:<pack-id>:<preset-id>` identifiers; they cannot overwrite built-ins.

Installing a different file with the same pack ID requires reviewing and replacing the installed pack.
The server checks the previewed revision to protect against concurrent replacements.
Stories save the resolved preset, so removing or replacing a pack does not restyle existing work.
Explicitly choosing a preset overrides the project style for that story. Choose Project style to use the saved project look.

API: GET /style-presets; POST /preset-packs/preview; POST /preset-packs/import;
GET /preset-packs/{id}/export; DELETE /preset-packs/{id}.
Import and preview accept the pack JSON directly. Replacement and deletion require an `If-Match` header containing the current pack hash from preview/catalog.
GET /preset-packs/starter/export downloads the six built-in presets as an editable starter pack.
