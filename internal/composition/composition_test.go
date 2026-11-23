package composition

import (
	"bytes"
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

// A render reads a revision's clips while the same edit can be saved again
// (the editor, or a second "finish the video"). The renderer must only ever see
// whole clips: a clip cut short mid-render froze the rest of that scene.
func TestSavingARevisionAgainNeverShowsAReaderAPartialClip(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "scene.mp4")
	clip := bytes.Repeat([]byte("frame-data"), 800_000)
	os.WriteFile(source, clip, 0600)
	m := Manifest{Version: 1, Width: 720, Height: 1280, FPS: 24, Duration: 10, Clips: []Clip{{ID: "scene", AssetID: "scene", Duration: 10, Speed: 1}}}
	sources := map[string]string{"scene": source}
	revision, err := Save(root, "project", m, sources)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := Load(root, "project", revision)
	staged := filepath.Join(root, "project", revision, saved.Clips[0].Path)

	done := make(chan struct{})
	short := make(chan int, 1)
	go func() {
		defer close(short)
		for {
			select {
			case <-done:
				return
			default:
			}
			if got, e := os.ReadFile(staged); e == nil && len(got) != len(clip) {
				short <- len(got)
				return
			}
		}
	}()
	for i := 0; i < 40; i++ {
		if again, e := Save(root, "project", m, sources); e != nil || again != revision {
			t.Fatal(again, e)
		}
	}
	close(done)
	if n, cut := <-short; cut {
		t.Fatalf("a reader saw the clip cut to %d of %d bytes while the revision was saved again", n, len(clip))
	}
}
func TestStillImageCanBeHeldLongerThanWhenAdded(t *testing.T) {
	root := t.TempDir()
	still := filepath.Join(root, "still.png")
	os.WriteFile(still, []byte("png"), 0600)
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 5, Clips: []Clip{{ID: "still", AssetID: "asset-1", Duration: 4.5, OriginalDuration: 3, Speed: 1}}}
	if _, err := Save(root, "project", m, map[string]string{"asset-1": still}); err != nil {
		t.Fatal("a still added at 3s should stretch to 4.5s:", err)
	}
}
func TestRoundedClipLengthStillSaves(t *testing.T) {
	root := t.TempDir()
	voice := filepath.Join(root, "voice.mp3")
	os.WriteFile(voice, []byte("mp3"), 0600)
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 6, Audio: []Clip{{ID: "v", AssetID: "asset-1", Duration: 5.042, OriginalDuration: 5.0416, Speed: 1}}}
	if _, err := Save(root, "project", m, map[string]string{"asset-1": voice}); err != nil {
		t.Fatal("a millisecond-rounded voice clip should save:", err)
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

func TestClipEndingAMillisecondPastTheLengthSavesButFarPastIsExplained(t *testing.T) {
	root := t.TempDir()
	voice := filepath.Join(root, "voice.mp3")
	os.WriteFile(voice, []byte("mp3"), 0600)
	sources := map[string]string{"asset-1": voice}
	near := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 15.802, Audio: []Clip{{ID: "v", AssetID: "asset-1", Start: 12.3456, Duration: 3.4567, OriginalDuration: 3.4567, Speed: 1}}}
	if _, err := Save(root, "project", near, sources); err != nil {
		t.Fatal("a clip ending under a millisecond past a rounded length should save:", err)
	}
	far := near
	far.Audio = []Clip{{ID: "v", AssetID: "asset-1", Start: 12, Duration: 5, OriginalDuration: 5, Speed: 1}}
	_, err := Save(root, "project", far, sources)
	if err == nil || !strings.Contains(err.Error(), "after the end of the video") {
		t.Fatalf("a clip far past the end should be refused with a clear reason, got %v", err)
	}
	speed := near
	speed.Audio = []Clip{{ID: "v", AssetID: "asset-1", Duration: 2, OriginalDuration: 2, Speed: 2}}
	if _, err = Save(root, "project", speed, sources); err == nil || !strings.Contains(err.Error(), "only normal speed") {
		t.Fatalf("a speed other than 1 should say so, got %v", err)
	}
}
