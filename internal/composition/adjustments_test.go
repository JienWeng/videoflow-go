package composition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdjustments(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.png")
	os.WriteFile(source, []byte("pixels"), 0600)
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 2, Clips: []Clip{{ID: "clip", AssetID: "asset", Duration: 2, Speed: 1, Muted: true, Adjustments: &Adjustments{Fit: "cover", Scale: 1.5, X: 25, Y: 60, Brightness: 1.1, Contrast: 1.2, Saturation: .8, Opacity: .9}}}}
	rev, err := Save(root, "project", m, map[string]string{"asset": source})
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := Load(root, "project", rev)
	html := Document(saved)
	for _, part := range []string{"object-fit:cover", "scale(1.5)", "brightness(1.1)", "opacity:0.9"} {
		if !strings.Contains(html, part) {
			t.Fatal("missing adjustment", part)
		}
	}
	m.Clips[0].Adjustments.Scale = 10
	if _, err = Save(root, "project", m, map[string]string{"asset": source}); err == nil {
		t.Fatal("invalid zoom accepted")
	}
	m.Clips[0].Adjustments.Scale = 1
	m.Clips[0].Adjustments.Fit = `cover;position:fixed`
	if _, err = Save(root, "project", m, map[string]string{"asset": source}); err == nil {
		t.Fatal("injected CSS accepted")
	}
}
