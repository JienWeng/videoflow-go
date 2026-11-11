package composition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallProjectSkills(t *testing.T) {
	workspace := t.TempDir()
	if e := InstallSkills(workspace); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{".agents/skills/hyperframes/SKILL.md", ".claude/skills/hyperframes/SKILL.md"} {
		b, e := os.ReadFile(filepath.Join(workspace, p))
		if e != nil || !strings.Contains(string(b), "VideoFlow override") {
			t.Fatal(p, e)
		}
	}
}
