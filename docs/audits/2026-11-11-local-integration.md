# Local production integration verification — 2026-11-11

Repository: JienWeng/videoflow-go. Base: d9ca833. No paid generation, native account inference or real provider submission was used for automated verification.

## Delivered paths

- All text planning/writing → selected genuine local Claude Code/Codex session, exact native ID resume, private instruction/skill workspace, persistent turns, interruption/review and cancellation controls.
- Full selected characters and assets → locked identity constraints, uploaded image references, stale-proposal rejection (including edits during a native turn), frozen video-job identity/voice policy and source validation at provider submission.
- Saved character voice default and explicit native reference mode → prepare/review/Run, durable receipts and uncertainty fences, speech/lip sync, exact approved script alignment, stable speaker/utterance/language cues, editable captions and SRT/VTT/JSON.
- Existing uploads/render outputs → versioned local HyperFrames composition, optional pinned runtime, review-only natural-language planning, clip/audio/caption forms, snapshots, local render and derived asset provenance.
- Create → generated keyframes pending consistency review → explicit approve/reject → explicit per-scene Render. Approval alone submits no video. Saved-voice renders are silent visual passes; speech/lip sync remain explicit voice stages. Native-reference profiles require their compatible workflow.

## Verification evidence

- `go test -race -count=1 ./...`: passed with real local FFmpeg/FFprobe available; provider and native inference fixtures are mocked.
- `go vet ./...`: passed.
- `npm run check`: zero errors/warnings; `npm run build`: passed.
- Real local HyperFrames lint/snapshot/MP4 rendering and import passed using pinned 0.9.33, Chrome and FFmpeg. No cloud rendering.
- Browser baseline: 36 checks on desktop/mobile; native session page: 8 checks. Before the additional consistency checkpoint, the clean integrated router/FFmpeg workflow passed 12 checks. Changed paths were then covered by 4 current identity/upload checks and 4 current reference-review checks, without pretending the earlier checkpoint was the final source.
- Composition forms, review-only planning, source invalidation and approved-caption import passed desktop/mobile browser tests. See [component coverage and reproducible commands](component-e2e-coverage.md) and [control inventory](component-e2e-controls.md). Inventoried controls are distinct from exercised assertions; reports disclose gaps.

## Independent review and fix pass

One fresh gpt-6-astra review found four Important issues and no confirmed Critical issue. One regression-driven fix pass addressed all four:

1. Ordinary video jobs omitted selected identity/voice policy: frozen render/voice policy tests and provider-boundary source validation tests passed; native profile misuse is rejected before submission.
2. Identity edits during a native turn blessed an old-context proposal: regression observed HTTP 200 before the fix and HTTP 409 afterward.
3. Native reference tasks bound file bytes but omitted semantic identity: metadata-only red→blue edits now invalidate Run with zero mocked paid submissions.
4. Generated keyframes were silently reused by paid video: API/worker tests and real-router browser tests prove no video calls until review plus explicit Render.

Additional observed regressions fixed: full story prompts cleared from cards; invalid text disguised as PNG attached as a character reference; approval buttons outside an oversized dialog; audio voice samples polluting visual references. New uploads return a public storage URL even under custom storage roots, verified against the actual static route. Field-specific recognition updates preserve identity locks changed during a vision request. Expanded contexts retain their original explicit selection for queue-time revalidation. Explicit retries retain frozen frames/voice policy rather than treating them as new manual overrides.

## Decisions and verification limits

- Retained the existing editor instead of adopting OpenCut during its rewrite; classic is archived. Cost: OpenCut-specific editing is deferred. No OpenCut adapter or lossless arbitrary HTML/GSAP round trip is claimed.
- Used documented exec JSONL/restricted native CLI proposals instead of experimental app-server dynamic tools/MCP. Cost: arbitrary native tool execution is intentionally unavailable; application Run controls execute work. Native JSONL is consumed, but token-by-token chat rendering is not provided.
- Parallelized independent feature/testing slices with separate file ownership and one final whole-change review. Cost: transient shared-tree compilation failures during RED/GREEN required combined verification before delivery.
- Live Claude/Codex authentication/account inference is a disclosed manual setup check. Claude is not installed on this verification host; installed Codex 0.165.0 flags were checked without inference. Cost: entitlement/login/version failures may still appear on a user's first real turn; no silent paid fallback occurs.
- Visual identity, voice similarity, actual spoken wording and lip-sync quality require human review and a separately approved live pilot. Cost: generative drift remains possible; mocks do not prove quality.
- Browser checks establish the explicitly recorded desktop/mobile scenarios, not every device, OS media decoder, race or possible UI combination. Cost: unlisted conditional combinations remain unverified. The optional runtime is not bundled into old published desktop ZIPs.

No deferred Minor findings were reported by the final reviewer. Source is verified; older published release packages are not upgraded by these source changes.
