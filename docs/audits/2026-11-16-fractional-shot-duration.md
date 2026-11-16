# Fractional shot-duration repair — 2026-11-16

The saved dog-story request stopped when AI returned duration 1.2: JSON decoding targeted ShotDraft.Duration (int), so ordinary numeric fractional output failed before validation. A regression test reproduced the identical error before implementation; decimal whole values such as 2.0 also failed.

## Fix

GenerateShots now decodes AI durations into a private numeric proposal, validates positive finite values, then allocates positive integer seconds across the requested scene total. Floor allocation reserves one second per remaining shot; remaining seconds go to the largest timing deficit with stable shot-order ties. This keeps the scene total exact, avoids zero-length shots and guards integer overflow. Saved ShotDraft/model/API/provider fields stay integers. Prompts request whole-second durations and a shot count that fits short scenes. Invalid/absent/null/string/nonpositive/nonfinite timings, overflowed totals and impossible shot counts are rejected. No automatic model retry.

## Evidence

- Before fix: TestGenerateShots reproduced `json: cannot unmarshal number 1.2 into Go struct field ShotDraft.shots.duration of type int`.
- Parser tests cover fractional timings, decimal integers, existing integer behavior, total preservation, subsecond minimums, invalid values and MaxInt overflow boundaries.
- A fake-native Codex dog-story pipeline returns duration 1.2, persists positive shot timings totaling the scene, creates storyboard images using a fixture provider, and stops at awaiting_reference_review with zero video submissions. Existing created-cast/identity and late-failure retention checks also pass.
- Full `go test ./... -count=1` passed in the existing media-enabled test-tools container. The initial plain Golang container lacked FFprobe and failed two voice tests; rerunning with installed media prerequisites passed every package. Focused tests also passed independently.
- Production backend Docker build passed. Diff whitespace check passed. Independent review is clean after adding minimum reservations for the extreme integer boundary.

No live provider/native inference, paid retry, stored-project mutation or sign-in changes. Backend deployment preserves data/auth volumes, after verifying zero running operations and renders. Existing failed-operation receipts are historical and remain visible; they are not rewritten into successes. The saved scene can continue through Generate shots (AI), then storyboard/reference review without recreating characters.

This resolves the separate parser issue recorded in `2026-11-16-story-progress.md`. Live model quality remains unverified.
