package composition

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gainAt is how loud the bed is at film time t: the sum of the pieces playing then,
// each at its volume times its linear edge fades. mediaAt is where in the file they are.
func gainAt(m Manifest, t float64) (gain float64, media []float64) {
	for _, a := range m.Audio {
		if !strings.HasPrefix(a.ID, BedPrefix) || t < a.Start || t >= a.Start+a.Duration {
			continue
		}
		g := *a.Volume
		if in := t - a.Start; a.FadeIn > 0 && in < a.FadeIn {
			g *= in / a.FadeIn
		}
		if out := a.Start + a.Duration - t; a.FadeOut > 0 && out < a.FadeOut {
			g *= out / a.FadeOut
		}
		gain += g
		media = append(media, a.Trim+t-a.Start)
	}
	return gain, media
}

func filmOfThreeClips() Manifest {
	m := Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 24, CaptionStyle: "default"}
	for i, id := range []string{"one", "two", "three"} {
		m.Clips = append(m.Clips, Clip{ID: id, AssetID: "clip-" + id, Start: float64(i * 8), Duration: 8, OriginalDuration: 8, Speed: 1, HasAudio: true})
	}
	return m
}

func TestOneMusicBedRunsUnderTheWholeFilmAndDucksUnderSpeech(t *testing.T) {
	speech := []Span{{2, 4.5}, {5, 6.5}, {11, 13}, {18.2, 20}}
	m, report, err := Score(filmOfThreeClips(), &Bed{AssetID: "music", Seconds: 10}, speech)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Music || !report.Continuous || !report.Looped || report.Ducks != 3 || report.CutFades != 2 || report.BedLevel != BedLevel || report.DuckLevel != DuckLevel {
		t.Fatalf("report %+v", report)
	}
	for t0 := 0.0; t0 < m.Duration; t0 += 0.01 {
		gain, media := gainAt(m, t0)
		// Overlapping pieces always play the same moment of the file, so the music runs
		// on unbroken while the level slides.
		for _, at := range media[1:] {
			if math.Abs(at-media[0]) > 0.002 {
				t.Fatalf("at %.2fs two pieces play different moments of the music: %v", t0, media)
			}
		}
		if t0 > FilmFadeIn && t0 < m.Duration-FilmFadeOut && gain < DuckLevel-0.001 && len(media) < 2 {
			t.Fatalf("the music drops out at %.2fs (gain %.3f)", t0, gain)
		}
		if len(media) == 0 && t0 > 0.001 {
			t.Fatalf("no music at %.2fs", t0)
		}
	}
	for _, s := range speech {
		for t0 := s.Start; t0 <= s.End; t0 += 0.05 {
			if gain, _ := gainAt(m, t0); math.Abs(gain-DuckLevel) > 0.002 {
				t.Fatalf("the music is at %.3f while someone speaks at %.2fs", gain, t0)
			}
		}
	}
	if gain, _ := gainAt(m, 15.5); math.Abs(gain-BedLevel) > 0.002 {
		t.Errorf("the music does not come back up between lines: %.3f", gain)
	}
	if gain, _ := gainAt(m, 0.1); gain >= BedLevel*0.5 {
		t.Errorf("the film does not fade the music in: %.3f", gain)
	}
	if gain, _ := gainAt(m, 23.95); gain >= DuckLevel {
		t.Errorf("the film does not fade the music out: %.3f", gain)
	}
	// The pause between 4.5 and 5 s is too short to come back up.
	if gain, _ := gainAt(m, 4.75); math.Abs(gain-DuckLevel) > 0.002 {
		t.Errorf("the music pumps between two close lines: %.3f", gain)
	}
	// Cuts between clips with their own sound get short fades on both sides.
	if m.Clips[0].FadeOut != CutFade || m.Clips[1].FadeIn != CutFade || m.Clips[1].FadeOut != CutFade || m.Clips[2].FadeIn != CutFade || m.Clips[0].FadeIn != 0 || m.Clips[2].FadeOut != 0 {
		t.Errorf("cut fades %+v", m.Clips)
	}

	// Saving checks every piece; it is an ordinary edit.
	dir := t.TempDir()
	sources := map[string]string{}
	for _, id := range []string{"clip-one", "clip-two", "clip-three"} {
		sources[id] = filepath.Join(dir, id+".mp4")
		os.WriteFile(sources[id], []byte(id), 0600)
	}
	sources["music"] = filepath.Join(dir, "music.wav")
	os.WriteFile(sources["music"], []byte("music"), 0600)
	root := t.TempDir()
	rev, err := Save(root, "project", m, sources)
	if err != nil {
		t.Fatalf("the scored film is refused: %v", err)
	}
	saved, _ := Load(root, "project", rev)
	if again := Inspect(saved); !again.Music || !again.Continuous || again.Ducks != 3 {
		t.Errorf("saved edit reads as %+v", again)
	}

	// Scoring again replaces the bed instead of adding a second one, and keeps the
	// owner's own tracks.
	volume := 0.8
	m.Audio = append(m.Audio, Clip{ID: "door", AssetID: "door", Start: 3, Duration: 1, Speed: 1, HasAudio: true, Volume: &volume})
	pieces := len(m.Audio) - 1
	again, _, err := Score(m, &Bed{AssetID: "music", Seconds: 10}, speech)
	if err != nil || len(again.Audio) != pieces+1 {
		t.Fatalf("rescoring: %v, %d tracks, want %d", err, len(again.Audio), pieces+1)
	}
	none, report, _ := Score(m, nil, speech)
	if len(none.Audio) != 1 || report.Music {
		t.Errorf("removing the bed left %d tracks", len(none.Audio))
	}
}

func TestBedWithoutSpeechAndShortTracks(t *testing.T) {
	m, report, err := Score(filmOfThreeClips(), &Bed{AssetID: "music", Seconds: 60}, nil)
	if err != nil || report.Pieces != 1 || report.Ducks != 0 || !report.Continuous {
		t.Fatalf("%v %+v", err, report)
	}
	bed := m.Audio[0]
	if bed.FadeIn != FilmFadeIn || bed.FadeOut != FilmFadeOut || *bed.Volume != BedLevel || bed.Duration != 24 {
		t.Errorf("bed %+v", bed)
	}
	if _, _, err = Score(filmOfThreeClips(), &Bed{AssetID: "music", Seconds: 2}, nil); err == nil {
		t.Error("a 2-second track was looped under a film")
	}
	// Speech from the first second keeps the music down from the start.
	m, _, _ = Score(filmOfThreeClips(), &Bed{AssetID: "music", Seconds: 60}, []Span{{0.3, 3}})
	if gain, _ := gainAt(m, 1); gain > DuckLevel+0.001 {
		t.Errorf("music over the first line: %.3f", gain)
	}
}
