package composition

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeRejectsUnapprovedCommand(t *testing.T) {
	if _, err := Run(context.Background(), t.TempDir(), "publish"); err == nil {
		t.Fatal("publish accepted")
	}
}

// Opt-in real integration: no network/model/provider calls. Uses an existing
// local fixture and the explicitly configured, pinned HyperFrames executable.
func TestLocalRenderIntegration(t *testing.T) {
	fixture := os.Getenv("VIDEOFLOW_HF_TEST_MEDIA")
	if fixture == "" {
		t.Skip("set VIDEOFLOW_HF_TEST_MEDIA for local renderer integration")
	}
	root := t.TempDir()
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, Clips: []Clip{{ID: "source", AssetID: "a", Duration: 2, Speed: 1}}, Captions: []Cue{{ID: "caption-1", Start: .2, End: 1.5, Text: "The approved red car."}}}
	rev, err := Save(root, "project", m, map[string]string{"a": fixture})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "project", rev)
	for _, op := range []string{"lint", "snapshot", "render"} {
		out, e := Run(context.Background(), dir, op)
		if e != nil {
			t.Fatalf("%s: %v\n%s", op, e, out)
		}
	}
	stat, e := os.Stat(filepath.Join(dir, "render.mp4"))
	if e != nil || stat.Size() == 0 {
		t.Fatal("no rendered output", e)
	}
}
