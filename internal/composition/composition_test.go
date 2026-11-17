package composition

import (
	"os"
	"os/exec"
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

func TestAudioGainAndFadesAreExportedAndValidated(t *testing.T) {
	source := filepath.Join(t.TempDir(), "music.wav")
	os.WriteFile(source, []byte("fixture"), 0600)
	volume := 0.15
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 8, Audio: []Clip{{ID: "music", AssetID: "sound", Duration: 8, Speed: 1, HasAudio: true, Volume: &volume, FadeIn: 1, FadeOut: 2}}}
	root := t.TempDir()
	rev, e := Save(root, "project", m, map[string]string{"sound": source})
	if e != nil {
		t.Fatal(e)
	}
	saved, e := Load(root, "project", rev)
	if e != nil {
		t.Fatal(e)
	}
	doc := Document(saved)
	for _, attr := range []string{`data-volume="0.15"`, `data-fade-in="1"`, `data-fade-out="2"`} {
		if !strings.Contains(doc, attr) {
			t.Fatal("export lost audio mix", attr, doc)
		}
	}
	volume = 1.2
	if _, e = Save(root, "project", m, map[string]string{"sound": source}); e == nil {
		t.Fatal("invalid volume accepted")
	}
	volume = 0
	m.Audio[0].FadeIn = 9
	if _, e = Save(root, "project", m, map[string]string{"sound": source}); e == nil {
		t.Fatal("fade exceeds clip")
	}
}

// Opt-in local verification fixture, never provider-generated media.
func TestWriteAudioExportVerificationFixture(t *testing.T) {
	root := os.Getenv("VIDEOFLOW_AUDIO_VERIFY_DIR")
	if root == "" {
		t.Skip("optional local export fixture")
	}
	os.MkdirAll(root, 0700)
	for _, cmd := range [][]string{{"-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=144x256", "-frames:v", "1", "-threads", "1", "-y", filepath.Join(root, "frame.png")}, {"-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "4", "-y", filepath.Join(root, "tone.wav")}} {
		if out, e := exec.Command("ffmpeg", cmd...).CombinedOutput(); e != nil {
			t.Fatal(e, string(out))
		}
	}
	gain := 0.15
	m := Manifest{Version: 1, Width: 144, Height: 256, FPS: 24, Duration: 4, Clips: []Clip{{ID: "frame", AssetID: "frame", Duration: 4, Speed: 1}}, Audio: []Clip{{ID: "music", AssetID: "music", Duration: 4, Speed: 1, HasAudio: true, Volume: &gain, FadeIn: 0.4, FadeOut: 1}}}
	rev, e := Save(root, "verification", m, map[string]string{"frame": filepath.Join(root, "frame.png"), "music": filepath.Join(root, "tone.wav")})
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "revision.txt"), []byte(rev), 0600)
}
