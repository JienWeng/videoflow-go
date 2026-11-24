package composition

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// EditorSkill is the vendored videoflow-editor skill: what the AI editing session may
// change in an edit, its hard rules and the proposal it hands back.
//
//go:embed editor/videoflow-editor/SKILL.md
var EditorSkill string

// InstallEditorSkills puts the vendored skills of an AI editing session (the pinned
// HyperFrames skills and videoflow-editor) in an app-owned workspace. They are copied
// from the binary; nothing is fetched or installed from elsewhere.
func InstallEditorSkills(workspace string) error {
	if err := InstallSkills(workspace); err != nil {
		return err
	}
	for _, location := range []string{".agents/skills", ".claude/skills"} {
		dir := filepath.Join(workspace, location, "videoflow-editor")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(EditorSkill), 0600); err != nil {
			return fmt.Errorf("install the videoflow-editor skill: %w", err)
		}
	}
	return nil
}
