package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A character's voice sample is sent with a reference-mode clip in which the character
// speaks, and bound to the character as @Audio 1. A character who says nothing in the
// clip sends none, and a removed sample is no longer sent.
func TestSpeakingCharactersSendTheirVoiceSample(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	tone := func(seconds string) []byte {
		file := filepath.Join(t.TempDir(), "tone.wav")
		if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration="+seconds, "-y", file).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
		data, _ := os.ReadFile(file)
		return data
	}
	spoken := ""
	old := voiceoverSpeech
	voiceoverSpeech = func(_ context.Context, provider, _, _, model, voice, text string) ([]byte, error) {
		if provider != "fish" || voice != "visitor1" {
			t.Fatalf("unexpected speech call %s %s", provider, voice)
		}
		spoken = text
		return tone("3"), nil
	}
	t.Cleanup(func() { voiceoverSpeech = old })

	s, scene := referenceScene(t)
	s.cfg.FishAPIKey = "test"
	project := *scene.ProjectID
	var cast []string
	_ = json.Unmarshal(scene.CharacterIDsJSON, &cast)
	chen, visitor := cast[0], cast[1]
	doc := map[string]any{}
	_ = json.Unmarshal(scene.SceneJSON, &doc)
	doc["spoken_lines"] = []SpokenLine{{ID: "hello", CharacterID: chen, Text: "The city never sleeps", Language: "English", Visible: true}}
	scene.SceneJSON = voiceJSON(doc)
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	upload := func(character string, audio []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("file", "voice.wav")
		part.Write(audio)
		form.Close()
		r := httptest.NewRequest("POST", "/voice/samples/"+character, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		return w
	}
	if w := upload(chen, tone("1")); w.Code != 400 || !strings.Contains(w.Body.String(), "at least 2 seconds") {
		t.Fatal("a one-second sample was accepted", w.Code, w.Body.String())
	}
	if w := upload(chen, []byte("not audio")); w.Code != 400 {
		t.Fatal("a file that is not audio was accepted", w.Code)
	}
	if w := upload(chen, tone("9")); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	sample := s.voiceSample(project, chen)
	if sample == nil {
		t.Fatal("sample was not saved")
	}
	full, _ := s.storagePath(sample.FilePath)
	if seconds, err := voiceRequireMedia(context.Background(), full, "audio"); err != nil || seconds > voiceSampleSeconds+0.5 || !strings.HasSuffix(full, ".mp3") {
		t.Fatalf("sample is not a short MP3: %v %v %s", seconds, err, full)
	}

	// The Visitor's sample is spoken by their saved voice; it needs one first.
	if w := call(t, s, "POST", "/voice/samples/"+visitor, ""); w.Code != 400 || !strings.Contains(w.Body.String(), "save a voice") {
		t.Fatal("a sample was made with no saved voice", w.Code, w.Body.String())
	}
	if _, e := s.database.SaveVoiceDocument(project, "profile", visitor, 0, voiceJSON(VoiceProfile{CharacterID: visitor, Provider: "fish", Model: "s1", VoiceID: "visitor1", Approved: true})); e != nil {
		t.Fatal(e)
	}
	if w := call(t, s, "POST", "/voice/samples/"+visitor, ""); w.Code != 200 || !strings.Contains(spoken, "The Visitor") {
		t.Fatal(w.Code, w.Body.String(), spoken)
	}
	var state struct {
		Samples map[string]string `json:"samples"`
	}
	if w := call(t, s, "GET", "/voice/state", ""); json.Unmarshal(w.Body.Bytes(), &state) != nil || len(state.Samples) != 2 {
		t.Fatal(w.Body.String())
	}

	render := func() (voices []string, names []string, prompt string) {
		t.Helper()
		before, _ := s.database.ListRenderJobs(project)
		if w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, scene.ID)); w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		<-finishPendingVideos(t, s, project, 1)
		jobs, _ := s.database.ListRenderJobs(project)
		if len(jobs) != len(before)+1 {
			t.Fatalf("jobs: %d", len(jobs))
		}
		var request struct {
			Final  string   `json:"final_prompt"`
			Audio  []string `json:"audio_urls"`
			Voices []string `json:"reference_voices"`
		}
		for _, job := range jobs {
			known := false
			for _, b := range before {
				known = known || b.ID == job.ID
			}
			if !known {
				_ = json.Unmarshal(job.RequestJSON, &request)
			}
		}
		return request.Audio, request.Voices, request.Final
	}
	audio, names, prompt := render()
	// Only Chen Wei is in this clip and speaks in it.
	if len(audio) != 1 || len(names) != 1 || names[0] != "Chen Wei" || !strings.Contains(prompt, "Chen Wei speaks with the voice in @Audio 1") || strings.Contains(prompt, "@Audio 2") {
		t.Fatalf("voice reference: %d files %v\n%s", len(audio), names, prompt)
	}

	if w := call(t, s, "DELETE", "/voice/samples/"+chen, ""); w.Code != 204 || s.voiceSample(project, chen) != nil {
		t.Fatal("sample was not removed", w.Code)
	}
	if _, err := os.Stat(full); err == nil {
		t.Fatal("the removed sample's file is still stored")
	}
}
