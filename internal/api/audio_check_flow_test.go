package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// nativeClipFixture is a finished 8-second Omni clip made with its own sound, whose
// scene has two Chinese lines.
func nativeClipFixture(t *testing.T, s *Server, project string, sound bool, script ...string) (*models.Scene, *models.RenderOutput) {
	t.Helper()
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{
		{"id": "l1", "text": "他从一万年后回来，", "language": "Chinese"},
		{"id": "l2", "text": "守住这台机器，不要让任何人靠近。", "language": "Chinese"},
	}})
	scene := &models.Scene{ProjectID: &project, Title: "Guard", Summary: "He guards the machine", Duration: 8, AspectRatio: "9:16", SceneJSON: sceneJSON, CharacterIDsJSON: json.RawMessage(`[]`)}
	if len(script) > 0 {
		scene.ScriptID = &script[0]
	}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"generate_audio": sound, "render_plan_id": "p1", "render_unit": map[string]any{"key": "p1:0", "index": 0, "count": 1, "length": 8, "duration": 8, "prompt": "He stands guard"}})
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "gemini", Status: models.StatusSucceeded, RequestJSON: request}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	video := "storage/outputs/" + job.ID + ".mp4"
	full, _ := s.storagePath(video)
	os.MkdirAll(filepath.Dir(full), 0700)
	os.WriteFile(full, []byte("not really a video "+job.ID), 0600)
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: video, QAJSON: json.RawMessage(`{"self_check":{"kind":"clip","result":{"verdict":"pass","score":90,"issues":[]},"hash":"x"}}`)}
	if err := s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	return scene, out
}

// cutOffSpeech is the first line said whole and the second stopped by the clip's end.
func cutOffSpeech() []providers.SpokenWord {
	return spoken("他從", 0.6, 1.0, "一萬年", 1.0, 1.6, "後", 1.6, 1.8, "回來", 1.8, 2.4,
		"守住", 4.2, 4.6, "这台", 4.6, 5.0, "机器", 5.0, 5.4, "不要让", 5.6, 6.6, "任", 7.6, 7.9)
}

// fakeListener stands in for the listening service and counts how often it is asked.
func fakeListener(t *testing.T, words []providers.SpokenWord, err error) *int {
	t.Helper()
	calls := 0
	old := listenToClip
	t.Cleanup(func() { listenToClip = old })
	listenToClip = func(_ *Server, _ context.Context, _, language, hint string) ([]providers.SpokenWord, float64, error) {
		calls++
		if language != "" || !strings.Contains(hint, "守住这台机器") {
			t.Errorf("listener asked with language %q hint %q", language, hint)
		}
		return words, 8, err
	}
	return &calls
}

func TestSpeechCheckEndpointStoresOnTheClipAndListensOnce(t *testing.T) {
	s, project := selfCheckServer(t)
	listened := fakeListener(t, cutOffSpeech(), nil)
	_, out := nativeClipFixture(t, s, project, true)

	w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID))
	var answer struct {
		Kind      string          `json:"kind"`
		Label     string          `json:"label"`
		Checked   bool            `json:"checked"`
		SelfCheck selfCheckRecord `json:"self_check"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || !answer.Checked || answer.Kind != selfCheckClipAudio || *listened != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := answer.SelfCheck.Result
	if r.Verdict != verdictFix || r.Issues[0].Code != "speech_cut_off" || r.Audio == nil || r.Audio.Listener != "openai" || r.Audio.Cost <= 0 || r.Audio.Cost > 0.01 || answer.SelfCheck.Engine != "openai" || answer.Label != "“Guard” clip’s speech" {
		t.Fatalf("%+v", answer)
	}
	saved, _ := s.database.GetRenderOutput(out.ID)
	if storedAudioCheck(saved.QAJSON) == nil || storedSelfCheck(saved.QAJSON) == nil || savedHearing(saved.QAJSON, out.VideoPath) == nil {
		t.Fatalf("every note should be kept: %s", saved.QAJSON)
	}
	// Unchanged clip and script: the stored answer, no new listening.
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.Checked || *listened != 1 {
		t.Fatal(w.Body.String())
	}
	// Check again re-judges from what was already heard: still no new listening.
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q,"force":true}`, out.ID))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || !answer.Checked || *listened != 1 {
		t.Fatal(w.Body.String())
	}
	w = call(t, s, "GET", "/selfcheck?kind=clip_audio&id="+out.ID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"stale":false`) || !strings.Contains(w.Body.String(), `"status":"cut_off"`) {
		t.Fatal(w.Body.String())
	}
	entries, _ := s.database.ListCosts(project)
	if len(entries) != 1 || entries[0].Label != "Listening to a clip" {
		t.Fatalf("one listening, paid once: %+v", entries)
	}

	// The review stage names the clip and what is wrong, in plain words.
	m, err := s.projectStages(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, st := range m.Stages {
		if st.ID != stageReview {
			continue
		}
		found = st.Status == statusBlocked && len(st.Findings) == 2 && st.Findings[1].Kind == selfCheckClipAudio && strings.Contains(strings.Join(st.Missing, "|"), "clip 1 (“Guard”): speech cut off at 7.9 s")
		if !found {
			t.Fatalf("%+v", st)
		}
	}
	if !found {
		t.Fatal("no review stage")
	}
	progress, _ := s.projectProgress(context.Background(), project)
	in, _ := s.stageInputs(context.Background(), project, progress)
	flags := flaggedClips(in.Clips)
	if len(flags) != 1 || flags[0].Kind != selfCheckClipAudio {
		t.Fatalf("%+v", flags)
	}
	d := clipReviewDecision(flags[0])
	if d.Options[0].Label != "Re-render with a shorter line" || d.Options[1].Label != "Keep it" || d.Key != selfCheckDecisionKey(out.ID) || !strings.Contains(d.Question, "speech check") || strings.Contains(d.Options[1].Value, "speech") {
		t.Fatalf("%+v", d)
	}

	// Keep it: no longer flagged.
	if w = call(t, s, "POST", "/selfcheck/accept", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	in, _ = s.stageInputs(context.Background(), project, progress)
	if len(flaggedClips(in.Clips)) != 0 {
		t.Fatal("a kept clip stays flagged")
	}
}

// The captions of a checked clip come from what was heard, without listening again.
func TestCaptionsReuseTheSpeechCheckTranscript(t *testing.T) {
	s, project := selfCheckServer(t)
	listened := fakeListener(t, cutOffSpeech(), nil)
	_, out := nativeClipFixture(t, s, project, true)
	if w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"clips": []composition.Clip{{ID: "c1", AssetID: "output-" + out.ID, Start: 10, Duration: 8, Speed: 1}}})
	w := call(t, s, "POST", "/compositions/captions/from-script", string(body))
	var got struct {
		Captions []composition.Cue `json:"captions"`
		Timing   map[string]int    `json:"timing"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || *listened != 1 || len(got.Captions) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	first, second := got.Captions[0], got.Captions[1]
	if first.Timing != timingHeard || first.Start != 10.52 || first.End != 12.7 || first.Text != "他从一万年后回来，" || first.Scripted != "" {
		t.Fatalf("first %+v", first)
	}
	// Cut off: the caption shows what was actually said, the script kept beside it.
	if second.Timing != timingHeard || second.Start != 14.12 || second.End != 18 || second.Scripted == "" || second.Text != "守住这台机器不要让任" {
		t.Fatalf("second %+v", second)
	}
	if got.Timing[timingHeard] != 2 || got.Timing["changed"] != 1 {
		t.Fatalf("%v", got.Timing)
	}
}

func TestSpeechCheckNeverBlocks(t *testing.T) {
	s, project := selfCheckServer(t)
	listened := fakeListener(t, nil, errNoListener)
	_, out := nativeClipFixture(t, s, project, true)
	w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, out.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unavailable":true`) || !strings.Contains(w.Body.String(), "AI setup (Listening)") || !strings.Contains(w.Body.String(), `"verdict":"pass"`) || *listened != 1 {
		t.Fatal(w.Body.String())
	}
	// Without a key the captions keep their estimated times, and say so.
	body, _ := json.Marshal(map[string]any{"clips": []composition.Clip{{ID: "c1", AssetID: "output-" + out.ID, Duration: 8, Speed: 1}}})
	if w = call(t, s, "POST", "/compositions/captions/from-script", string(body)); w.Code != 200 || !strings.Contains(w.Body.String(), `"estimated":2`) {
		t.Fatal(w.Body.String())
	}
	// A clip made without its own sound has nothing to hear: no listening at all.
	_, silent := nativeClipFixture(t, s, project, false)
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip_audio","id":%q}`, silent.ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "without its own sound") || *listened != 2 {
		t.Fatal(w.Body.String(), *listened)
	}
}

// Full auto listens to each clip made with its own sound and asks about a flagged one;
// it never makes a video again.
func TestFullAutoListensToClipsAndAsksNeverRemakes(t *testing.T) {
	s, project := selfCheckServer(t)
	defer fakeFrames(t, s)()
	calls, _ := selfCheckCLI(t, s, map[string][]string{"clip": {passAnswer}})
	listened := fakeListener(t, cutOffSpeech(), nil)
	scene, out := nativeClipFixture(t, s, project, true)
	jobsBefore, _ := s.database.ListRenderJobs(project)
	runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{SceneIDs: []string{scene.ID}, VideosOnly: true, FullAuto: true}, scenes: []models.Scene{*scene}}
	ctx := checkContext(t, s, project)
	flagged := runner.checkClips(ctx)
	jobsAfter, _ := s.database.ListRenderJobs(project)
	if len(flagged) != 1 || flagged[0].Kind != selfCheckClipAudio || flagged[0].ID != out.ID || calls("clip") != 1 || *listened != 1 || len(jobsAfter) != len(jobsBefore) {
		t.Fatalf("flagged %+v frames %d listened %d jobs %d→%d", flagged, calls("clip"), *listened, len(jobsBefore), len(jobsAfter))
	}
	result := runner.clipReviewResult(flagged)
	d := result["decision"].(chatDecision)
	if d.Options[0].Label != "Re-render with a shorter line" || d.Reason != askExpensive || !strings.Contains(d.Question, "Speech cut off at 7.9 s") {
		t.Fatalf("%+v", d)
	}
	// Kept in the chat: not flagged again, nothing listened to again.
	s.recordAnswer(project, &chatAnswer{Key: d.Key, Question: d.Question, Label: "Keep it", Value: d.Options[1].Value})
	if len(runner.checkClips(ctx)) != 0 || *listened != 1 {
		t.Fatal("a kept clip was flagged or listened to again")
	}
	// Manual and Auto never listen by themselves.
	runner.body.FullAuto = false
	if len(runner.checkClips(ctx)) != 0 || *listened != 1 {
		t.Fatal("only Full auto checks by itself")
	}
}

// Full auto's export captions the joined video from what was heard in each clip, so
// the exported captions match the sound; the clip is listened to once for the speech
// check and the captions together.
func TestFullAutoExportCaptionsFollowTheHeardSpeech(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	// Lining the scenes up reads the video model list (free); nothing else may be called.
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			http.NotFound(w, r)
			return
		}
		t.Errorf("the export must never call a paid service: %s", r.URL.Path)
	})
	project, _ := s.database.GetActiveProjectID()
	if _, err := s.database.CreateDirectorSession(project, "codex", "Checks"); err != nil {
		t.Fatal(err)
	}
	defer fakeFrames(t, s)()
	selfCheckCLI(t, s, map[string][]string{"clip": {passAnswer}})
	listened := fakeListener(t, cutOffSpeech(), nil)
	script := &models.Script{ProjectID: &project, Idea: "guard", Title: "Guard", Summary: "guard", DraftJSON: []byte("{}")}
	if err := s.database.CreateScript(script); err != nil {
		t.Fatal(err)
	}
	scene, out := nativeClipFixture(t, s, project, true, script.ID)
	// A whole-scene take, as an older render or a single-clip scene makes.
	job, _ := s.database.GetRenderJob(out.RenderJobID)
	job.RequestJSON = json.RawMessage(`{"generate_audio":true}`)
	if err := s.database.UpdateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	full, _ := s.storagePath(out.VideoPath)
	if b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=90x160:r=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "8", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-y", full).CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{ScriptID: script.ID, FullAuto: true}, scenes: []models.Scene{*scene}}
	ctx := checkContext(t, s, project)
	if flagged := runner.checkClips(ctx); len(flagged) != 1 || *listened != 1 {
		t.Fatalf("%+v", flagged)
	}
	revision, err := runner.saveCaptionedComposition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m, err := composition.Load(s.compositionRoot(), project, revision)
	if err != nil || len(m.Captions) != 2 || *listened != 1 {
		t.Fatalf("%v %+v listened %d", err, m.Captions, *listened)
	}
	if c := m.Captions[0]; c.Timing != timingHeard || c.Start != 0.52 || c.End != 2.7 {
		t.Fatalf("exported caption not at the heard time: %+v", c)
	}
}
