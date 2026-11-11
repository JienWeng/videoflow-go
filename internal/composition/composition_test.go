package composition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevisionAndTimeline(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	os.WriteFile(source, []byte("pixels"), 0600)
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, Clips: []Clip{{ID: "clip-1", AssetID: "asset-1", Duration: 2, Speed: 1}}, Captions: []Cue{{ID: "cue-1", Start: .2, End: 1.5, Text: "<approved>"}}}
	revision, err := Save(root, "project", m, map[string]string{"asset-1": source})
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(root, "project", revision, "index.html"))
	if err != nil || !strings.Contains(string(html), "&lt;approved&gt;") {
		t.Fatalf("unsafe/missing HTML: %s %v", html, err)
	}
	got, err := Load(root, "project", revision)
	if err != nil || got.Clips[0].SHA256 == "" {
		t.Fatal(got, err)
	}
	m.Clips[0].Trim = 1
	if _, err = Save(root, "project", m, map[string]string{"asset-1": source}); err == nil {
		t.Fatal("accepted trim beyond original duration")
	}
}
func TestRejectsTraversalAndUnsupportedSpeed(t *testing.T) {
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, Clips: []Clip{{ID: "x", AssetID: "a", Duration: 2, Speed: 2}}}
	if _, err := Save(t.TempDir(), "../escape", m, nil); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := Save(t.TempDir(), "safe", m, nil); err == nil {
		t.Fatal("unsupported speed accepted")
	}
}
func TestVerifyRejectsEditedSource(t *testing.T) {
	root := t.TempDir()
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2}
	revision, err := Save(root, "project", m, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := Load(root, "project", revision)
	dir := filepath.Join(root, "project", revision)
	if err = Verify(dir, saved); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("tampered"), 0600)
	if Verify(dir, saved) == nil {
		t.Fatal("source mutation accepted")
	}
}

func TestRejectsUnsupportedCaptionStyle(t *testing.T) {
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, CaptionStyle: "unsupported"}
	if _, e := Save(t.TempDir(), "project", m, nil); e == nil {
		t.Fatal("unsupported caption style silently accepted")
	}
}
