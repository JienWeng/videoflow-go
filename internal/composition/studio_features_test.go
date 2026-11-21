package composition

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStudioTextOverlayRoundTripAndSafety(t *testing.T) {
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, Clips: []Clip{}, Audio: []Clip{}, Captions: []Cue{}, Texts: []TextLayer{{ID: "title", Text: "<script>alert('no')</script>", Start: 0, End: 2, Size: 48, Color: "#FFFFFF", Background: "#112233", Font: "serif", Position: "center"}}}
	root := t.TempDir()
	revision, e := Save(root, "project", m, map[string]string{})
	if e != nil {
		t.Fatal(e)
	}
	saved, e := Load(root, "project", revision)
	if e != nil || len(saved.Texts) != 1 {
		t.Fatal(e, saved)
	}
	html := Document(saved)
	if strings.Contains(html, "<script>alert") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("unsafe title", html)
	}
	if e = Verify(filepath.Join(root, "project", revision), saved); e != nil {
		t.Fatal(e)
	}
	m.Texts[0].Color = "red;position:fixed"
	if _, e = Save(root, "project", m, map[string]string{}); e == nil {
		t.Fatal("unsafe brand color accepted")
	}
	if (OverlayBox{X: 90, Y: 0, Width: 20, Height: 20}).Valid() {
		t.Fatal("out of bounds overlay")
	}
}
