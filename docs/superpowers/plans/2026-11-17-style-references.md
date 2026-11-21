# Style references

Implement the user's image/video/link starting point through existing project style and vision routing. Analyze an uploaded Library asset or a safely fetched public HTTPS image/video; a web page can supply its public Open Graph image. Sample video frames locally with FFmpeg and label this limitation. Reject private network destinations, invalid/oversized inputs, unsupported media and missing web previews before inference. Reference content is untrusted data, never tool instructions.

Analysis returns an editable draft only. The user explicitly saves it as the project default before choosing it in the creative brief. Show name, visual description, palette, lighting, tone and audience. Preserve existing story snapshots and generation confirmation. This is visual analysis, not model training or automatic reproduction of copyrighted content.

- [x] REF-01 Safe remote fetch, owned asset validation, bounded image/video sampling and draft endpoint.
- [x] REF-02 Shared upload/link/Library analysis UI, editable draft, explicit save, brief selection and chat attachment.
- [x] REF-03 Mock-provider API/browser/security regressions, full checks, PR update and deployment. No paid generation in tests.

Verified: full Go suite, Svelte zero errors/warnings, short-video FFmpeg decode, ownership/private-link tests, 16 desktop/mobile reference checks with two clean WCAG scans and zero browser errors, plus the previous creative-brief regression flow. Reference guidance: docs/STYLE-REFERENCES.md. No paid inference or generation.

Production Docker images built successfully. Delivered in PR #2 without merging; live health and Tailscale route verified after deployment.
