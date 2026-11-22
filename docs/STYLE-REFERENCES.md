# Starting from visual references

Open the paperclip in Chat, the reference section in the brief's Look & mood step, Project styling, or Library's Derive visual style action. All use one shared panel. Upload an image/video, choose Library media, or paste a public HTTPS link. Sending a bare link or a link described as a style/reference/look in chat opens the panel without starting inference.

Choose Analyze reference to use the configured vision API. It can consume credits. This extracts visible visual direction rather than training a model. Review/edit the style name, visual direction, palette, lighting, suggested tone and audience. Save as project style explicitly replaces the default for future stories. Saved story snapshots and locked identities remain intact. The creative brief must still be confirmed before Create.

An active chat brief refreshes through the selected AI session after saving, retaining the original idea and other choices and obtaining a current proposal. Merely analyzing does not modify it. Reference analysis itself uses the vision API, not Codex/Claude text inference.

## Input limits

- PNG, JPEG, GIF and WebP images; MP4/MOV/WebM video; at most 50 MB.
- Video analysis uses at most four frames sampled roughly five seconds apart from the beginning. It does not analyze audio, precise pacing, or every scene. Short clips can yield one frame.
- Direct public media links are downloaded without credentials. Web pages use only their public Open Graph/Twitter image, not a browser screenshot or complete video. Upload a clip/screenshot for private pages, unsupported sharing sites or pages without a usable preview.
- Private, loopback, link-local and Tailscale destinations are rejected. DNS destinations and redirects are checked by the existing protected downloader. Uploaded assets must belong to the active project.
- Media is decoded locally with bounded FFmpeg execution and local-file-only protocols. Web scripts are not executed. Extracted guidance remains editable; no model training or media generation is performed by analysis.

## API and verification

`POST /style/reference` accepts exactly one of `asset_id` or `url`, plus optional `instruction`. It supports the existing background-operation contract. The result contains `style`, `source`, `source_kind`, `sample_count` and `saved:false`. Saving uses the existing project-scoped style endpoint; the UI detects a changed project/style before replacement.

Run `go test ./... -count=1` with FFmpeg, frontend `npm run check`, and production builds. `scripts/e2e_style_references.mjs` mocks every API/provider response and covers desktop/mobile link entry, draft-only analysis, manual edits, explicit saving, sampled-video disclosure, image upload, one active refreshed brief, confirmation and accessibility. `scripts/e2e_creative_brief.mjs` checks the previous brief workflow still works. No live paid inference or generation is used for verification.
