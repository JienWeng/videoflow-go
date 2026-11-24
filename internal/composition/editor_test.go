package composition

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorSkillsAreVendoredInBothSkillFolders(t *testing.T) {
	dir := t.TempDir()
	if err := InstallEditorSkills(dir); err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{".agents/skills", ".claude/skills"} {
		editor, err := os.ReadFile(filepath.Join(dir, location, "videoflow-editor", "SKILL.md"))
		if err != nil || !strings.Contains(string(editor), "Never cut inside a spoken line") || !strings.Contains(string(editor), "out/proposal.json") {
			t.Fatalf("%s: %v", location, err)
		}
		if _, err = os.Stat(filepath.Join(dir, location, "hyperframes", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListeningIsASettingAndNeverDownloads(t *testing.T) {
	t.Setenv("VIDEOFLOW_EARSHOT_LISTEN", "")
	if _, _, err := Listen(context.Background(), "film.mp4"); err == nil || !strings.Contains(err.Error(), "listening is off") {
		t.Fatal(err)
	}
	env := strings.Join(EarshotEnvironment(), " ")
	if !strings.Contains(env, "EARSHOT_OFFLINE=1") || !strings.Contains(env, "HF_HUB_OFFLINE=1") {
		t.Fatal(env)
	}
	t.Setenv("VIDEOFLOW_EARSHOT_BIN", "relative/earshot")
	if _, err := Earshot(); err == nil {
		t.Fatal("a relative earshot path was accepted")
	}
}
