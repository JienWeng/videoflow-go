package api

import (
	"context"
	"encoding/json"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Captions in the finished film show the words actually heard, when they are heard.

type captionFixture struct {
	t       *testing.T
	s       *Server
	project string
	scene   *models.Scene
}

func newCaptionFixture(t *testing.T, heard []providers.SpokenWord) *captionFixture {
	s, _ := falServer(t, "done")
	project, _ := s.database.GetActiveProjectID()
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{
		{"id": "l1", "text": "It won't fly. I give up.", "language": "en"},
		{"id": "l2", "text": "Wait. I know a secret.", "language": "en"},
	}})
	scene := &models.Scene{ProjectID: &project, Title: "Kite", Duration: 10, AspectRatio: "9:16", SceneJSON: sceneJSON}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	old := listenToClip
	t.Cleanup(func() { listenToClip = old })
	listenToClip = func(*Server, context.Context, string, string, string) ([]providers.SpokenWord, float64, error) {
		return heard, 10, nil
	}
	return &captionFixture{t, s, project, scene}
}

func (f *captionFixture) clip(sound bool) string {
	request, _ := json.Marshal(map[string]any{"generate_audio": sound, "render_unit": map[string]any{"key": "plan:0", "count": 1, "length": 10, "duration": 10}})
	job := &models.RenderJob{ProjectID: &f.project, SceneID: &f.scene.ID, Status: "succeeded", RequestJSON: request}
	if e := f.s.database.CreateRenderJob(job); e != nil {
		f.t.Fatal(e)
	}
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/outputs/" + job.ID + ".mp4"}
	if e := f.s.database.CreateRenderOutput(out); e != nil {
		f.t.Fatal(e)
	}
	return "output-" + out.ID
}

func (f *captionFixture) asset(id, kind string, meta any) {
	raw, _ := json.Marshal(meta)
	a := &models.Asset{ID: id, ProjectID: &f.project, Type: kind, Name: id, FilePath: "storage/assets/" + id + ".mp3", TagsJSON: []byte("[]"), MetadataJSON: raw}
	if e := f.s.database.CreateUploadedAsset(a); e != nil {
		f.t.Fatal(e)
	}
}

func (f *captionFixture) captions(clips, audio []composition.Clip, spokenOnly bool) scriptCaptionResult {
	return f.s.scriptCaptions(f.t.Context(), f.project, clips, audio, spokenOnly)
}

func bothLinesHeard() []providers.SpokenWord {
	return spoken("It", 3.2, 3.4, "won't", 3.4, 3.7, "fly", 3.7, 4.0, "I", 4.3, 4.4, "give", 4.4, 4.6, "up", 4.6, 4.9,
		"Wait", 7.0, 7.4, "I", 7.6, 7.7, "know", 7.7, 7.9, "a", 7.9, 8.0, "secret", 8.0, 8.6)
}

// The clip's own voice says the script: captions show the script, at the heard words.
func TestCaptionsShowTheScriptAtTheHeardWords(t *testing.T) {
	f := newCaptionFixture(t, bothLinesHeard())
	res := f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Start: 20, Duration: 10, Speed: 1}}, nil, true)
	c := res.Captions
	if len(c) != 2 || len(res.Dropped) != 0 || c[0].Text != "It won't fly. I give up." || c[0].Start != 23.12 || c[0].End != 25.2 || c[0].Timing != timingHeard ||
		c[1].Text != "Wait. I know a secret." || c[1].Start != 26.92 || c[1].Timing != timingHeard || c[1].Scripted != "" {
		t.Fatalf("%+v %+v", c, res.Dropped)
	}
}

// Different words were said: the caption shows what was heard, the script kept aside.
func TestCaptionsShowWhatWasSaidWhenItDiffers(t *testing.T) {
	f := newCaptionFixture(t, spoken("It", 3.2, 3.4, "won't", 3.4, 3.7, "fly", 3.7, 4.0, "I", 4.3, 4.4, "give", 4.4, 4.6, "up", 4.6, 4.9,
		"Wait", 7.0, 7.4, "I", 7.6, 7.7, "have", 7.7, 7.9, "a", 7.9, 8.0, "plan", 8.0, 8.6, "now", 8.6, 8.9))
	res := f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Duration: 10, Speed: 1}}, nil, true)
	if len(res.Captions) != 2 || res.Captions[1].Scripted != "Wait. I know a secret." || res.Captions[1].Text == res.Captions[1].Scripted || res.Timing["changed"] != 1 {
		t.Fatalf("%+v", res.Captions)
	}
}

// A line the clip was listened to for and did not say is never captioned.
func TestALineNotHeardIsNotCaptioned(t *testing.T) {
	f := newCaptionFixture(t, spoken("It", 3.2, 3.4, "won't", 3.4, 3.7, "fly", 3.7, 4.0, "I", 4.3, 4.4, "give", 4.4, 4.6, "up", 4.6, 4.9))
	res := f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Duration: 10, Speed: 1}}, nil, false)
	if len(res.Captions) != 1 || res.Captions[0].UtteranceID != "l1" || len(res.Dropped) != 1 || res.Dropped[0]["utterance_id"] != "l2" ||
		res.Dropped[0]["reason"] != "it was not heard in the clip's sound" || res.Timing["dropped"] != 1 {
		t.Fatalf("%+v %+v", res.Captions, res.Dropped)
	}
	// A voice line that says it still gives it a caption, on the voice.
	f.asset("voiceover_l2", "audio", map[string]any{"voiceover": true, "text": "Wait, I know a secret!"})
	res = f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Duration: 10, Speed: 1}}, []composition.Clip{{ID: "v", AssetID: "voiceover_l2", Start: 6, Duration: 2.5, Speed: 1}}, true)
	if len(res.Captions) != 2 || res.Captions[1].Timing != timingVoice || res.Captions[1].Start != 6 || res.Captions[1].End != 8.5 || res.Captions[1].Text != "Wait, I know a secret!" || len(res.Dropped) != 0 {
		t.Fatalf("%+v %+v", res.Captions, res.Dropped)
	}
}

// Voice-over replaces the clip's sound: captions follow the voice lines placed, never
// the muted clip's own speech; a line no voice says is left out.
func TestReplacedVoiceCaptionsFollowTheVoiceLines(t *testing.T) {
	f := newCaptionFixture(t, bothLinesHeard())
	f.asset("voiceover_a", "audio", map[string]any{"voiceover": true, "text": "It won't fly. I give up."})
	clips := []composition.Clip{{ID: "c", AssetID: f.clip(true), Start: 10, Duration: 10, Speed: 1, Muted: true}}
	audio := []composition.Clip{{ID: "v", AssetID: "voiceover_a", Start: 11, Duration: 2, Speed: 1}}
	res := f.captions(clips, audio, true)
	if len(res.Captions) != 1 || res.Captions[0].Timing != timingVoice || res.Captions[0].Start != 11 || res.Captions[0].End != 13 || res.Followed != 1 {
		t.Fatalf("%+v", res.Captions)
	}
	if len(res.Dropped) != 1 || res.Dropped[0]["utterance_id"] != "l2" || res.Dropped[0]["reason"] != "its clip is muted and no voice line in the edit speaks it" {
		t.Fatalf("%+v", res.Dropped)
	}
	// A muted voice line speaks nothing.
	audio[0].Muted = true
	if res = f.captions(clips, audio, true); len(res.Captions) != 0 || len(res.Dropped) != 2 {
		t.Fatalf("%+v", res.Captions)
	}
}

// A voice-first clip plays its dialogue track: captions take each line's place in the
// track (shifted by the track's place and trim), whatever the clip itself was heard saying.
func TestVoiceFirstCaptionsFollowTheDialogueTrack(t *testing.T) {
	f := newCaptionFixture(t, spoken("Something", 1.0, 1.5, "else", 1.5, 2.0))
	f.asset("voicetrack_x", "voice_track", map[string]any{"voice_track": true, "seconds": 6, "lines": []voiceTrackLine{
		{LineID: "l1", Text: "It won't fly. I give up.", Start: 0.25, Duration: 2},
		{LineID: "l2", Text: "Wait. I know a secret.", Start: 2.55, Duration: 2},
	}})
	clips := []composition.Clip{{ID: "c", AssetID: f.clip(true), Start: 30, Duration: 10, Speed: 1, Muted: true}}
	audio := []composition.Clip{{ID: "voice-c", AssetID: "voicetrack_x", Start: 30, Trim: 0.25, Duration: 6, Speed: 1}}
	res := f.captions(clips, audio, true)
	if len(res.Captions) != 2 || len(res.Dropped) != 0 || res.Followed != 2 {
		t.Fatalf("%+v %+v", res.Captions, res.Dropped)
	}
	a, b := res.Captions[0], res.Captions[1]
	if a.Timing != timingVoice || a.Text != "It won't fly. I give up." || a.Start != 30 || a.End != 32.3 ||
		b.Timing != timingVoice || b.Text != "Wait. I know a secret." || b.Start != 32.22 || b.End != 34.6 {
		t.Fatalf("%+v %+v", a, b)
	}
}

// Trims and offsets in the edit move captions with the clip; a line mostly trimmed away
// is not captioned.
func TestCaptionsFollowClipTrims(t *testing.T) {
	f := newCaptionFixture(t, bothLinesHeard())
	// Trimmed 4.5 s in: line 1 (3.2-4.9 s) is mostly gone; line 2 (7.0-8.6) plays at 2.5 s.
	res := f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Start: 5, Trim: 4.5, Duration: 5.5, Speed: 1}}, nil, true)
	if len(res.Captions) != 1 || res.Captions[0].UtteranceID != "l2" || res.Captions[0].Start != 7.42 || res.Captions[0].End != 9.4 {
		t.Fatalf("%+v", res.Captions)
	}
	if len(res.Dropped) != 1 || res.Dropped[0]["reason"] != "most of it is trimmed out of the edit" {
		t.Fatalf("%+v", res.Dropped)
	}
	// At double speed the heard times halve.
	res = f.captions([]composition.Clip{{ID: "c", AssetID: f.clip(true), Start: 0, Duration: 5, Speed: 2}}, nil, true)
	if len(res.Captions) != 2 || res.Captions[1].Start != 3.46 || res.Captions[1].End != 4.45 {
		t.Fatalf("%+v", res.Captions)
	}
}

// A clip without its own sound and no voice line speaks nothing: the finished film
// leaves its lines uncaptioned, while the editor keeps them as estimates.
func TestSilentClipLinesAreNotCaptionedInTheFinishedFilm(t *testing.T) {
	f := newCaptionFixture(t, nil)
	clips := []composition.Clip{{ID: "c", AssetID: f.clip(false), Duration: 10, Speed: 1}}
	if res := f.captions(clips, nil, false); len(res.Captions) != 2 || res.Captions[0].Timing != timingEstimated {
		t.Fatalf("editor: %+v", res.Captions)
	}
	if res := f.captions(clips, nil, true); len(res.Captions) != 0 || len(res.Dropped) != 2 {
		t.Fatalf("film: %+v", res.Captions)
	}
}
