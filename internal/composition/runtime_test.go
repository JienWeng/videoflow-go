package composition

import (
	"context"
	"os"
	"os/exec"
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

func TestLocalAdjustedRenderIntegration(t *testing.T) {
	fixture := os.Getenv("VIDEOFLOW_HF_TEST_MEDIA")
	if fixture == "" {
		t.Skip("set VIDEOFLOW_HF_TEST_MEDIA for local renderer integration")
	}
	root := t.TempDir()
	m := Manifest{Version: 1, Width: 320, Height: 180, FPS: 12, Duration: 1, Clips: []Clip{{ID: "source", AssetID: "a", Duration: 1, Speed: 1, Adjustments: &Adjustments{Fit: "cover", Scale: 1.1, X: 50, Y: 50, Brightness: 0, Contrast: 1, Saturation: 1, Opacity: 1}}}}
	rev, e := Save(root, "project", m, map[string]string{"a": fixture})
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "project", rev)
	if log, e := Run(context.Background(), dir, "render"); e != nil {
		t.Fatal(e, log)
	}
	pixel, e := exec.Command("ffmpeg", "-v", "error", "-i", filepath.Join(dir, "render.mp4"), "-vf", "scale=1:1", "-pix_fmt", "gray", "-frames:v", "1", "-f", "rawvideo", "pipe:1").Output()
	if e != nil || len(pixel) != 1 || pixel[0] > 8 {
		t.Fatalf("brightness adjustment missing in rendered pixels: %v %v", pixel, e)
	}
}
