package composition

import (
	"context"
	"os"
	"os/exec"
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
func TestEditedAudioMixRespectsTimeline(t *testing.T) {
	binary, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("FFmpeg required")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.wav")
	if out, e := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=400:duration=3", source).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	gain := .15
	m := Manifest{Version: 1, Duration: 2, Audio: []Clip{{ID: "sound", AssetID: "owned", Start: .5, Trim: 1, Duration: 1, Speed: 1, Volume: &gain, FadeIn: .2, FadeOut: .2}}}
	dest := filepath.Join(dir, "mix.wav")
	if e = MixAudio(context.Background(), m, map[string]string{"owned": source}, dest); e != nil {
		t.Fatal(e)
	}
	duration, _, _, audio, e := Probe(context.Background(), dest)
	if e != nil || duration < 1.99 || duration > 2.01 || !audio {
		t.Fatal(duration, audio, e)
	}
	data, _ := os.ReadFile(dest)
	if len(data) < 50000 {
		t.Fatal("no actual mix produced")
	}
	m.Audio[0].Trim = 3
	if e = MixAudio(context.Background(), m, map[string]string{"owned": source}, dest); e == nil {
		t.Fatal("invalid source trim accepted")
	}
}
