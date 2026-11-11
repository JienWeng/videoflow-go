package composition

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed reviewed
var reviewedSkills embed.FS

const SkillOverride = `
## VideoFlow override — authoritative application workflow

This project uses HyperFrames 0.9.33, upstream source e596be16c901639ce6c18032e56924a9d8e1967d (Apache-2.0; LICENSE is included). The following application instructions override every upstream default below and in references.

Use existing approved project assets and the VideoFlow version-1 editing manifest. Inspect project state via app tools. Propose changes to clips, audio and explicit approved script caption cues. Keep original asset IDs and hashes, source revision and editable source links. The supported manifest renderer handles normal-speed media clips and timed captions; unsupported arbitrary HTML/GSAP edits require a separate source workflow and cannot be promised as lossless editor interchange.

Only app-controlled local lint/check/snapshot/render operations are allowed through the composition Run endpoint, bound to the exact saved source revision. Do not use raw shell, npx, upstream auto-updaters, telemetry events, usage/auth calls, registry fetches or upstream skill installers. Do not generate media, transcribe, align, create voices/music, publish, share or render in the cloud. Existing fal/BytePlus/OpenRouter routing and explicit application Run confirmation govern paid actions; never retry a paid action automatically. Missing source media or timing requires review. Script wording is authoritative, final spoken audio supplies timing, untimed text remains a draft. Do not invent evenly-spaced caption timings. Scaffold only with skip-transcribe and skip-skills if explicitly authorized by the app.

Both Codex and Claude Code use these identical project-local skills. Do not change global agent configuration or install additional skills. The reviewed upstream source below is retained for authoring reference; its service and execution defaults are superseded above. OpenCut has no validated adapter here.

---
`

// InstallSkills installs pinned, vendored authoring references in app-owned
// workspaces only. Callers must supply their managed project workspace.
func InstallSkills(workspace string) error {
	for _, location := range []string{".agents/skills", ".claude/skills"} {
		err := fs.WalkDir(reviewedSkills, "reviewed", func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			rel := strings.TrimPrefix(p, "reviewed/")
			target := filepath.Join(workspace, location, rel)
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			b, e := reviewedSkills.ReadFile(p)
			if e != nil {
				return e
			}
			if filepath.Base(p) == "SKILL.md" {
				s := string(b)
				if strings.HasPrefix(s, "---\n") {
					if end := strings.Index(s[4:], "\n---"); end >= 0 {
						end += 8
						s = s[:end] + "\n" + SkillOverride + s[end:]
					}
				}
				b = []byte(s)
			}
			return os.WriteFile(target, b, 0600)
		})
		if err != nil {
			return fmt.Errorf("install pinned HyperFrames project skills: %w", err)
		}
	}
	return nil
}

// AgentInstructions provides the reviewed authoring contract to tool-disabled
// native planners. Discovery files alone would be unreadable without file tools.
func AgentInstructions() (string, error) {
	core, err := reviewedSkills.ReadFile("reviewed/hyperframes/SKILL.md")
	if err != nil {
		return "", err
	}
	return SkillOverride + "\nReviewed HyperFrames authoring reference (application override takes precedence):\n" + string(core), nil
}
