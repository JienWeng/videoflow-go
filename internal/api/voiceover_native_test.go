package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/providers"
)

// Slots and speed-ups are worked out in milliseconds: a slot is never rounded up into
// the next line, and the speed-up is rounded up so the faster line fits.
func TestVoiceSlotsAndTempoRoundToTheMillisecond(t *testing.T) {
	items := []voiceoverItem{
		{Text: "b", Start: 14.2, Duration: 3.1, clipEnd: 18},
		{Text: "a", Start: 10.6, Duration: 3.1, clipEnd: 18},
		{Text: "typed", Start: 20, Duration: 9},
	}
	long := fitVoiceover(items)
	if items[0].Text != "a" || items[0].Slot != 3.45 || items[1].Slot != 3.8 || items[2].Slot != 0 {
		t.Fatalf("slots: %+v", items)
	}
	if len(long) != 0 {
		t.Fatalf("3.1 s fits 3.45 s and 3.8 s: %v", long)
	}
	items[0].Duration = 3.751
	if long = fitVoiceover(items); len(long) != 1 || long[0] != 0 {
		t.Fatalf("long: %v", long)
	}
	for _, c := range []struct{ duration, slot, want float64 }{
		{3.751, 3.45, 1.088}, // 1.08724… rounds up
		{2.851, 2.85, 1.001},
		{2.85, 2.85, 1},
		{3.45, 3, 1.15},
		{4, 3, 1.334},
	} {
		if got := neededTempo(c.duration, c.slot); got != c.want {
			t.Errorf("neededTempo(%v, %v) = %v, want %v", c.duration, c.slot, got, c.want)
		}
		if got := neededTempo(c.duration, c.slot); c.duration/got > c.slot+1e-9 {
			t.Errorf("tempo %v leaves %v s in a %v s slot", got, c.duration/got, c.slot)
		}
	}
	if !math.IsInf(neededTempo(1, 0), 1) {
		t.Fatal("no slot needs an endless speed-up")
	}
}

func tone(t *testing.T, seconds float64) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tone.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=330:duration=%g", seconds), "-ac", "1", "-ar", "44100", "-y", path).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	data, _ := os.ReadFile(path)
	return data
}

// Replacing the clip's own voices mutes the clip, speaks each line when the character
// says it in the clip, and speeds a line up (at most 1.15×) instead of overlapping;
// keeping them voices nothing the clip already says. What it makes saves as an edit.
func TestVoiceoverReplacesNativeVoicesAtTheHeardTimes(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	s, project := selfCheckServer(t)
	s.cfg.FishAPIKey = "test"
	fakeListener(t, cutOffSpeech()[:4], nil) // line 1 heard 0.6-2.4 s
	_, out := nativeClipFixture(t, s, project, true)
	// Line 2 is said too, from 3.5 s: line 1's slot is 3.5-0.15-0.6 = 2.75 s.
	listenToClip = func(*Server, context.Context, string, string, string) ([]providers.SpokenWord, float64, error) {
		return append(cutOffSpeech()[:4], spoken("守住这台机器", 3.5, 5.0, "不要让任何人靠近", 5.2, 6.8)...), 8, nil
	}
	mp3 := tone(t, 3.1)
	speech := 0
	old := voiceoverSpeech
	voiceoverSpeech = func(context.Context, string, string, string, string, string, string) ([]byte, error) {
		speech++
		return mp3, nil
	}
	t.Cleanup(func() { voiceoverSpeech = old })
	clip := composition.Clip{ID: "c1", AssetID: "output-" + out.ID, Start: 10, Duration: 8, OriginalDuration: 8, Speed: 1, HasAudio: true}
	narrator := map[string]string{"provider": "fish", "voice_id": "narrator1", "model": "s1"}
	type result struct {
		Items    []voiceoverItem  `json:"items"`
		Mute     []string         `json:"mute_clip_ids"`
		Native   []string         `json:"native_clip_ids"`
		Kept     int              `json:"kept_native"`
		TooLong  []map[string]any `json:"too_long"`
		SpedUp   int              `json:"sped_up"`
		Shifted  int              `json:"shifted"`
		Mode     string           `json:"mode"`
		Captions []composition.Cue
	}
	ask := func(body map[string]any) result {
		raw, _ := json.Marshal(body)
		w := call(t, s, "POST", "/compositions/voiceover", string(raw))
		var r result
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return r
	}

	// Not checked yet: replacing is offered first. Once heard saying its lines, keeping.
	status := func() map[string]any {
		raw, _ := json.Marshal(map[string]any{"clips": []any{clip}})
		w := call(t, s, "POST", "/compositions/voiceover/status", string(raw))
		var r map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return r
	}
	if st := status(); st["recommended"] != "replace" || st["unchecked"] != 1.0 {
		t.Fatalf("%v", st)
	}
	if w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID)); w.Code != 200 || !strings.Contains(w.Body.String(), `"verdict":"pass"`) {
		t.Fatal(w.Body.String())
	}
	if st := status(); st["recommended"] != "keep" || st["verified"] != 1.0 {
		t.Fatalf("%v", st)
	}

	// Keep: the clip says its own lines, nothing is voiced or muted.
	kept := ask(map[string]any{"clips": []any{clip}, "narrator": narrator, "mode": "keep"})
	if len(kept.Items) != 0 || kept.Kept != 2 || len(kept.Mute) != 0 || len(kept.Native) != 1 || speech != 0 {
		t.Fatalf("keep: %+v", kept)
	}

	// Replace: muted, placed where heard; line 1 (3.1 s) does not fit its 2.75 s slot.
	r := ask(map[string]any{"clips": []any{clip}, "narrator": narrator, "mode": "replace"})
	if r.Mode != "replace" || len(r.Mute) != 1 || r.Mute[0] != "c1" || len(r.Items) != 2 || speech != 2 {
		t.Fatalf("replace: %+v", r)
	}
	if r.Items[0].Start != 10.6 || r.Items[0].Timing != timingHeard || r.Items[0].Slot != 2.75 || r.Items[1].Slot != 4.5 {
		t.Fatalf("not at the heard times: %+v", r.Items)
	}
	if len(r.TooLong) != 1 || r.TooLong[0]["fits_with_speed_up"] != true || r.TooLong[0]["tempo_needed"].(float64) > 1.15 || r.Shifted != 1 {
		t.Fatalf("too long: %+v shifted %d", r.TooLong, r.Shifted)
	}
	// With speed-up the long line is made faster and fits: one voice at a time, none moved.
	r = ask(map[string]any{"clips": []any{clip}, "narrator": narrator, "mode": "replace", "speed_up": true})
	if r.SpedUp != 1 || len(r.TooLong) != 0 || r.Shifted != 0 || speech != 2 {
		t.Fatalf("speed up: %+v", r)
	}
	first, second := r.Items[0], r.Items[1]
	if first.Tempo < 1.1 || first.Tempo > 1.15 || first.Duration > first.Slot || first.Start+first.Duration+voiceGap > second.Start+0.0005 || first.OriginalDuration < first.Duration {
		t.Fatalf("faster line: %+v next %+v", first, second)
	}
	// Asking again reuses the faster file.
	if again := ask(map[string]any{"clips": []any{clip}, "narrator": narrator, "mode": "replace", "speed_up": true}); again.Items[0].AssetID != first.AssetID || !again.Items[0].Cached {
		t.Fatalf("not reused: %+v", again.Items[0])
	}

	// The edit the editor makes from this saves: the clip muted, the voices on the
	// Audio track, the length rounded up to the millisecond like the editor does.
	paths, _, err := s.compositionSources(project)
	if err != nil {
		t.Fatal(err)
	}
	video := clip
	video.Muted = true
	m := composition.Manifest{Version: 1, Width: 720, Height: 1280, FPS: 24, Clips: []composition.Clip{video}}
	end := video.Start + video.Duration
	for i, item := range r.Items {
		m.Audio = append(m.Audio, composition.Clip{ID: fmt.Sprintf("voice-%d", i), AssetID: item.AssetID, Start: item.Start, Duration: item.Duration, OriginalDuration: item.OriginalDuration, Speed: 1, HasAudio: true})
		end = max(end, item.Start+item.Duration)
	}
	m.Duration = math.Ceil(end*1000) / 1000
	full, _ := s.storagePath(out.VideoPath)
	sources := map[string]string{clip.AssetID: full}
	for _, item := range r.Items {
		sources[item.AssetID] = paths[item.AssetID]
	}
	if _, err := composition.Save(t.TempDir(), project, m, sources); err != nil {
		t.Fatalf("the voiced edit does not save: %v", err)
	}

	// Captions follow the voice lines and show the script's words.
	raw, _ := json.Marshal(map[string]any{"clips": []any{clip}, "audio": m.Audio})
	w := call(t, s, "POST", "/compositions/captions/from-script", string(raw))
	var captions struct {
		Captions []composition.Cue `json:"captions"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &captions) != nil || len(captions.Captions) != 2 || captions.Captions[0].Timing != timingVoice || captions.Captions[0].Start != 10.6 {
		t.Fatal(w.Body.String())
	}
	if w = call(t, s, "POST", "/compositions/voiceover", `{"clips":[],"mode":"lower"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "keep") {
		t.Fatal("an unknown mode was accepted", w.Code)
	}
}
