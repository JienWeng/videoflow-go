package director

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexImageCapabilityAndRestrictedArguments(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "codex")
	os.WriteFile(cli, []byte("#!/bin/sh\nprintf 'image_generation stable true\\n'\n"), 0700)
	runner := &Runner{Codex: cli}
	if !runner.ImagesAvailable(context.Background()) {
		t.Fatal("native image capability not recognized")
	}
	args := strings.Join(CodexImageArgs([]string{"/tmp/ref.png"}), " ")
	if !strings.Contains(args, "features.image_generation=true") || !strings.Contains(strings.Join(CodexArgs(""), " "), "features.image_generation=false") || !strings.Contains(args, "features.shell_tool=false") || !strings.Contains(args, "--image /tmp/ref.png") || !strings.Contains(args, "--ephemeral") {
		t.Fatal(args)
	}
}
