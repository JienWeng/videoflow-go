package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/seedance"
)

func TestSoundModeIsAProjectSettingThatDefaultsToTogether(t *testing.T) {
	s := routerServer(t, nil)
	var view struct {
		SoundMode string           `json:"sound_mode"`
		Services  []map[string]any `json:"voice_video_services"`
	}
	w := call(t, s, "GET", "/sound-mode", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.SoundMode != soundTogether || len(view.Services) == 0 || view.Services[0]["id"] != "argolink" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(t, s, "PUT", "/sound-mode", `{"sound_mode":"loud"}`); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(t, s, "PUT", "/sound-mode", `{"sound_mode":"voice_first"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"sound_mode":"voice_first"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	if s.soundMode(project) != soundVoiceFirst {
		t.Fatal("sound mode not kept")
	}
	if d, _ := s.database.GetDecision(project, soundModeKey); d == nil || d.Source != db.DecisionByUser {
		t.Fatalf("decision = %+v", d)
	}
}

// fakeSpeech replaces the paid speech call with a short tone and counts the calls.
func fakeSpeech(t *testing.T, seconds string) *[]string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	sample := filepath.Join(t.TempDir(), "tone.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration="+seconds, "-y", sample).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	mp3, _ := os.ReadFile(sample)
	said := &[]string{}
	old := voiceoverSpeech
	voiceoverSpeech = func(_ context.Context, provider, _, _, _, voice, text string) ([]byte, error) {
		*said = append(*said, voice+":"+text)
		return mp3, nil
	}
	t.Cleanup(func() { voiceoverSpeech = old })
	return said
}

// voiceScene is a scene of two shots: Ada says l1 in shot-1, the narrator l2 in shot-2.
func voiceScene(t *testing.T, s *Server) (*models.Scene, string) {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	ada := &models.Character{ProjectID: &project, Name: "Ada"}
	if e := s.database.CreateCharacter(ada); e != nil {
		t.Fatal(e)
	}
	profile, _ := json.Marshal(VoiceProfile{CharacterID: ada.ID, Provider: "fish", VoiceID: "ada-voice", Model: "s1", Approved: true})
	if _, e := s.database.SaveVoiceDocument(project, "profile", ada.ID, 0, profile); e != nil {
		t.Fatal(e)
	}
	narrator, _ := json.Marshal(voiceChoice{Provider: "fish", VoiceID: "narrator1", Model: "s1"})
	if _, e := s.database.SetDecision(project, voiceTrackNarratorKey, string(narrator), db.DecisionByUser, false); e != nil {
		t.Fatal(e)
	}
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{
		{"id": "l1", "character_id": ada.ID, "text": "The towers are moving.", "shot_id": "shot-1"},
		{"id": "l2", "text": "And nobody noticed.", "shot_id": "shot-2"},
	}})
	scene := &models.Scene{ProjectID: &project, Title: "Towers", Duration: 8, AspectRatio: "9:16", SceneJSON: sceneJSON}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	return scene, ada.ID
}

func TestVoiceTracksAreMadePerClipFromPinnedLinesAndCached(t *testing.T) {
	said := fakeSpeech(t, "0.6")
	s := routerServer(t, nil)
	s.cfg.FishAPIKey = "test"
	scene, _ := voiceScene(t, s)
	first := renderUnit{Index: 0, ShotIDs: []string{"shot-1"}, Length: 4, Duration: 4}
	second := renderUnit{Index: 1, ShotIDs: []string{"shot-2"}, Length: 4, Duration: 4}

	// Reading never pays.
	if tr, err := s.voiceTrack(context.Background(), scene, first, 2, false); err != nil || tr.Ready || len(*said) != 0 {
		t.Fatalf("track = %+v err = %v said = %v", tr, err, *said)
	}
	a, err := s.voiceTrack(context.Background(), scene, first, 2, true)
	if err != nil || !a.Ready || len(a.Lines) != 1 || a.Lines[0].LineID != "l1" || a.Lines[0].Speaker != "Ada" {
		t.Fatalf("track = %+v err = %v", a, err)
	}
	if a.Seconds < 0.6 || a.Lines[0].Start != voiceTrackLead || a.URL == "" {
		t.Fatalf("timing = %+v", a)
	}
	b, err := s.voiceTrack(context.Background(), scene, second, 2, true)
	if err != nil || len(b.Lines) != 1 || b.Lines[0].LineID != "l2" || b.AssetID == a.AssetID {
		t.Fatalf("track = %+v err = %v", b, err)
	}
	if strings.Join(*said, "|") != "ada-voice:The towers are moving.|narrator1:And nobody noticed." {
		t.Fatalf("said = %v", *said)
	}
	// Asked again: the same tracks, nothing paid.
	again, _ := s.voiceTrack(context.Background(), scene, first, 2, false)
	if !again.Ready || !again.Cached || again.AssetID != a.AssetID || len(*said) != 2 {
		t.Fatalf("again = %+v said = %v", again, *said)
	}
	if asset, e := s.database.GetAsset(a.AssetID); e != nil || asset.Type != "voice_track" {
		t.Fatalf("asset = %+v %v", asset, e)
	}

	// A changed line is a new track for its clip only; the other line is not re-voiced.
	lines := sceneSpokenLines(scene)
	lines[1].Text = "And nobody noticed at all."
	raw, _ := json.Marshal(map[string]any{"spoken_lines": lines})
	scene.SceneJSON = raw
	if tr, _ := s.voiceTrack(context.Background(), scene, first, 2, false); !tr.Ready || tr.AssetID != a.AssetID {
		t.Fatalf("unchanged clip lost its track: %+v", tr)
	}
	if tr, _ := s.voiceTrack(context.Background(), scene, second, 2, false); tr.Ready {
		t.Fatalf("changed line kept the old track: %+v", tr)
	}
	if _, err = s.voiceTrack(context.Background(), scene, second, 2, true); err != nil || len(*said) != 3 {
		t.Fatalf("err = %v said = %v", err, *said)
	}
}

func TestVoiceTrackNeedsAVoiceForEverySpeaker(t *testing.T) {
	said := fakeSpeech(t, "0.3")
	s := routerServer(t, nil)
	scene, _ := voiceScene(t, s)
	project := *scene.ProjectID
	if _, e := s.database.DeleteDecision(project, voiceTrackNarratorKey); e != nil {
		t.Fatal(e)
	}
	_, err := s.voiceTrack(context.Background(), scene, renderUnit{ShotIDs: []string{"shot-2"}, Length: 4}, 2, true)
	if err == nil || !strings.Contains(err.Error(), "no voice for Narrator") || len(*said) != 0 {
		t.Fatalf("err = %v said = %v", err, *said)
	}
}

func TestVoiceFirstPlanCoversTheTrackAndNeedsAServiceThatTakesVoice(t *testing.T) {
	fakeSpeech(t, "5.4")
	s := routerServer(t, nil)
	s.cfg.FishAPIKey = "test"
	scene, _ := voiceScene(t, s)
	project := *scene.ProjectID
	units := []renderUnit{{Index: 0, ShotIDs: []string{"shot-1"}, Length: 4, Duration: 4}, {Index: 1, ShotIDs: []string{"shot-2"}, Length: 4, Duration: 4}}
	plan := func(voices int) productionPlan {
		p := productionPlan{Settings: productionSettings{AudioMode: "native"}, Units: append([]renderUnit{}, units...)}
		s.applyVoiceFirst(context.Background(), scene, &p, voices, []int{4, 5, 6, 8})
		return p
	}
	if p := plan(3); p.SoundMode != "" {
		t.Fatal("together mode must leave the plan alone")
	}
	if e := s.setSoundMode(project, soundVoiceFirst, db.DecisionByUser); e != nil {
		t.Fatal(e)
	}
	if p := plan(3); !strings.Contains(p.voiceFirstBlock, "make_voice_tracks") {
		t.Fatalf("block = %q", p.voiceFirstBlock)
	}
	if p := plan(0); p.voiceFirstBlock != voiceFirstServiceBlock {
		t.Fatalf("block = %q", p.voiceFirstBlock)
	}
	for _, u := range units {
		if _, err := s.voiceTrack(context.Background(), scene, u, 2, true); err != nil {
			t.Fatal(err)
		}
	}
	p := plan(3)
	if p.voiceFirstBlock != "" || len(p.VoiceTracks) != 2 || p.Units[0].Duration != 6 || p.Units[0].Length != 4 {
		t.Fatalf("plan = %+v", p)
	}
	if planVoiceTrack(p, 1) != p.VoiceTracks[1].AssetID || planVoiceTrack(p, 1) == "" {
		t.Fatal("render unit not given its track")
	}
	// The clip is sent the track, not the voice samples.
	payload := map[string]any{"voice_track": p.VoiceTracks[0].AssetID}
	if !s.attachVoiceTrack(payload, project) || len(payload["audio_urls"].([]string)) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	raw, _ := json.Marshal(payload)
	job := models.RenderJob{ProjectID: &project, RequestJSON: raw}
	if a, seconds := s.voiceTrackOfJob(job); a == nil || seconds < 5.4 {
		t.Fatalf("track of job = %+v %v", a, seconds)
	}
}

func TestVoiceTrackPromptLipSyncsToAudioOne(t *testing.T) {
	c := seedance.Clip{Family: "seedance-2.0", Summary: "Ada on a roof", Audio: seedance.Native, VoiceTrack: true,
		References: []seedance.Reference{{Kind: "character", Name: "Ada"}}, Shots: []seedance.Shot{{Seconds: 4, Action: "Ada looks up"}}}
	prompt := seedance.Compose(c)
	if !strings.Contains(prompt, "Lip-sync the speaking characters to @Audio 1 exactly") || strings.Contains(prompt, "@Audio 2") {
		t.Fatal(prompt)
	}
	if err := seedance.Check(c, prompt+" @Audio 2"); err == nil {
		t.Fatal("a second recording was accepted")
	}
}

func TestChatSetsVoiceFirstAndOffersAServiceThatTakesVoice(t *testing.T) {
	answer := `{"reply":"I'll make the speech first.","action":"set_sound_mode","sound_mode":"voice_first"}`
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	// Videos are made by OpenRouter (takes no voice); ArgoLink is connected.
	if e := s.database.SetProviderSecret("argolink", "test-key-argolink", nil); e != nil {
		t.Fatal(e)
	}
	s.database.SetAppSetting("default_video_provider", `"openrouter"`)
	s.database.SetAppSetting("video_model", `"bytedance/seedance-2.0"`)
	if p, _ := s.routes().MediaSelection("video"); hostTakesVoice(p) {
		t.Fatalf("setup: %s", p)
	}
	w := call(t, s, "POST", "/chat", `{"message":"I want to hear the lines before the video"}`)
	var out struct {
		Intent struct {
			Action    string         `json:"action"`
			SoundMode string         `json:"sound_mode"`
			Switch    map[string]any `json:"video_switch"`
		} `json:"intent"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if out.Intent.Action != setSoundModeAction || out.Intent.SoundMode != soundVoiceFirst || out.Intent.Switch["provider"] != "argolink" {
		t.Fatalf("intent = %+v", out.Intent)
	}
	if !strings.Contains(*prompt, `"sound_mode":{"sound_mode":"together"`) || !strings.Contains(*prompt, `"video_service_takes_voice":false`) {
		t.Fatal("planner not told the sound mode")
	}
	project, _ := s.database.GetActiveProjectID()
	if s.soundMode(project) != soundTogether {
		t.Fatal("the proposal changed the mode before the click")
	}
}

func TestVoiceTracksAreASceneChatAction(t *testing.T) {
	if canonicalChatAction(makeVoiceTracksAction) != makeVoiceTracksAction || canonicalChatAction(setSoundModeAction) != setSoundModeAction {
		t.Fatal("actions not on the card contract")
	}
	if !sceneScopedAction(makeVoiceTracksAction) || !selectedChatActionAllowed("scene", makeVoiceTracksAction) || !selectedChatActionAllowed("character", setSoundModeAction) {
		t.Fatal("selection rules")
	}
}

func TestSceneVoiceTracksEndpointReadsFreeAndMakesInTheBackground(t *testing.T) {
	said := fakeSpeech(t, "0.4")
	s := routerServer(t, nil)
	s.cfg.FishAPIKey = "test"
	scene, _ := voiceScene(t, s)
	if e := s.database.SetProviderSecret("argolink", "test-key-argolink", nil); e != nil {
		t.Fatal(e)
	}
	s.database.SetAppSetting("default_video_provider", `"argolink"`)
	s.database.SetAppSetting("video_model", `"seedance-2.0"`)
	for i, id := range []string{"shot-1", "shot-2"} {
		if e := s.database.CreateShot(&models.Shot{ID: id, SceneID: scene.ID, ShotOrder: i + 1, Duration: 4, Prompt: "p"}); e != nil {
			t.Fatal(e)
		}
	}
	w := call(t, s, "GET", "/scenes/"+scene.ID+"/voice-tracks", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":false`) || len(*said) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "POST", "/scenes/"+scene.ID+"/voice-tracks", `{"narrator":{"provider":"fish","voice_id":"bad id!"}}`)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "POST", "/scenes/"+scene.ID+"/voice-tracks", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"line_id":"l1"`) || len(*said) != 2 {
		t.Fatal(w.Code, w.Body.String(), *said)
	}
}
