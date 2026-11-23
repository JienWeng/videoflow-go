package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// retryFixture is a Full auto run over one 8-second Chinese scene made in one clip with
// its own sound, whose two lines the listener (fake) hears as it is told: heard[n] is
// what the n-th listening hears. The writer (fake) shortens both lines; making the clip
// again (fake video service) saves a new finished clip at once.
type retryFixture struct {
	s       *Server
	project string
	scene   *models.Scene
	out     *models.RenderOutput
	runner  *autopilotRunner
	ctx     context.Context
	heard   [][]providers.SpokenWord
	listens int
	writes  int
	renders []string
}

// shortenedSpeech is the shorter lines said whole.
func shortenedSpeech() []providers.SpokenWord {
	return spoken("他", 0.6, 0.8, "回来", 0.8, 1.4, "了", 1.4, 1.6, "守住", 4.2, 4.8, "机器", 4.8, 5.4)
}

func newRetryFixture(t *testing.T, full bool) *retryFixture {
	t.Helper()
	s, scene := productionFixture(t, "auto")
	project := *scene.ProjectID
	if _, err := s.database.CreateDirectorSession(project, "codex", "Checks"); err != nil {
		t.Fatal(err)
	}
	shots, _ := s.database.ListShots(scene.ID)
	scene.SceneJSON = voiceJSON(map[string]any{
		"production":       productionSettings{AudioMode: "native", Strategy: "auto"},
		"generation_brief": map[string]any{"language": "Chinese"},
		"spoken_lines": []SpokenLine{
			{ID: "l1", Language: "Chinese", Text: "他从一万年后回来，", ShotID: shots[0].ID},
			{ID: "l2", Language: "Chinese", Text: "守住这台机器，不要让任何人靠近。", ShotID: shots[2].ID},
		},
	})
	if err := s.database.UpdateScene(scene); err != nil {
		t.Fatal(err)
	}
	scene, _ = s.database.GetScene(scene.ID)
	f := &retryFixture{s: s, project: project, scene: scene}
	f.out = f.finishedClip(t, "old")
	t.Cleanup(fakeFrames(t, s))
	selfCheckCLI(t, s, map[string][]string{"clip": {passAnswer}})
	oldListen := listenToClip
	t.Cleanup(func() { listenToClip = oldListen })
	listenToClip = func(*Server, context.Context, string, string, string) ([]providers.SpokenWord, float64, error) {
		f.listens++
		words := f.heard[min(f.listens, len(f.heard))-1]
		if words == nil {
			return nil, 8, errNoListener
		}
		return words, 8, nil
	}
	s.agents.SetStructuredRunner(func(_ context.Context, name, _, _ string, target any) error {
		f.writes++
		if name != "speech_repair_agent" {
			t.Errorf("unexpected agent %s", name)
		}
		return json.Unmarshal([]byte(`{"lines":[{"id":"l1","character_id":"","text":"他回来了，","shot":1},{"id":"l2","character_id":"","text":"守住机器！","shot":3}]}`), target)
	})
	oldRender := speechRepairRender
	t.Cleanup(func() { speechRepairRender = oldRender })
	speechRepairRender = func(_ *Server, _ context.Context, sceneID string, quote speechRepairQuote) ([]string, error) {
		f.renders = append(f.renders, sceneID)
		out := f.finishedClip(t, quote.PlanID)
		return []string{out.RenderJobID}, nil
	}
	f.ctx = checkContext(t, s, project)
	f.runner = f.newRunner(full)
	return f
}

func (f *retryFixture) newRunner(full bool) *autopilotRunner {
	op := &models.Op{Kind: "autopilot", ProjectID: &f.project, Status: "running"}
	_ = f.s.database.CreateOp(op)
	scene, _ := f.s.database.GetScene(f.scene.ID)
	return &autopilotRunner{s: f.s, op: op, project: f.project, body: autopilotRequest{SceneIDs: []string{scene.ID}, VideosOnly: true, FullAuto: full}, scenes: []models.Scene{*scene}}
}

// finishedClip saves a finished one-clip render of the scene, made with its own sound.
func (f *retryFixture) finishedClip(t *testing.T, plan string) *models.RenderOutput {
	t.Helper()
	shots, _ := f.s.database.ListShots(f.scene.ID)
	ids := []string{}
	for _, sh := range shots {
		ids = append(ids, sh.ID)
	}
	request, _ := json.Marshal(map[string]any{"generate_audio": true, "render_plan_id": plan, "render_unit": map[string]any{"key": plan + ":0", "index": 0, "count": 1, "length": 8, "duration": 8, "shot_ids": ids}})
	job := &models.RenderJob{ProjectID: &f.project, SceneID: &f.scene.ID, Provider: "gemini", Status: models.StatusSucceeded, RequestJSON: request}
	if err := f.s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	video := "storage/outputs/" + job.ID + ".mp4"
	full, _ := f.s.storagePath(video)
	os.MkdirAll(filepath.Dir(full), 0700)
	os.WriteFile(full, []byte("not really a video "+job.ID), 0600)
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: video}
	if err := f.s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	return out
}

// check runs the run's clip check and its bounded speech retry, as run() does.
func (f *retryFixture) check(t *testing.T, runner *autopilotRunner) []selfCheckFlag {
	t.Helper()
	flagged := runner.checkClips(f.ctx)
	if len(flagged) == 0 {
		return nil
	}
	flagged, _, err := runner.retryClipSpeech(f.ctx, flagged)
	if err != nil {
		t.Fatal(err)
	}
	return flagged
}

// A flagged clip gets exactly one rewrite, one new clip and one more listening; when
// the new clip is heard whole, the run goes on, and the retry is in the decision log.
func TestFullAutoRetriesAFlaggedClipsSpeechOnce(t *testing.T) {
	f := newRetryFixture(t, true)
	f.heard = [][]providers.SpokenWord{cutOffSpeech(), shortenedSpeech()}
	if flagged := f.check(t, f.runner); len(flagged) != 0 {
		t.Fatalf("still flagged: %+v", flagged)
	}
	if f.writes != 1 || len(f.renders) != 1 || f.renders[0] != f.scene.ID || f.listens != 2 {
		t.Fatalf("writes %d renders %v listens %d", f.writes, f.renders, f.listens)
	}
	saved, _ := f.s.database.GetScene(f.scene.ID)
	if lines := sceneSpokenLines(saved); lines[1].Text != "守住机器！" {
		t.Fatalf("lines not saved: %+v", lines)
	}
	d, err := f.s.database.GetDecision(f.project, speechRetryKey(f.out.ID))
	if err != nil || d.Source != db.DecisionByAssistant || !strings.Contains(d.Value, "re-rendered it once") {
		t.Fatalf("decision: %+v %v", d, err)
	}
	// The assistant sees it in the chat context, to explain what it did.
	if items := chatDecidedSoFar([]db.ProjectDecision{*d})["decisions"].([]map[string]string); items[0]["key"] != speechRetryKey(f.out.ID) {
		t.Fatalf("%+v", items)
	}
}

// Still flagged after its retry: the run asks, saying it already tried once, and
// nothing more is made, in this run or a second one.
func TestFullAutoAsksAfterTheOneRetryAndNeverRetriesAgain(t *testing.T) {
	f := newRetryFixture(t, true)
	cutAgain := spoken("他", 0.6, 0.8, "回来", 0.8, 1.4, "了", 1.4, 1.6, "守", 7.6, 7.9)
	f.heard = [][]providers.SpokenWord{cutOffSpeech(), cutAgain}
	flagged := f.check(t, f.runner)
	if len(flagged) != 1 || !flagged[0].Retried || flagged[0].ID == f.out.ID {
		t.Fatalf("flagged %+v", flagged)
	}
	d := f.runner.clipReviewResult(flagged)["decision"].(chatDecision)
	if d.Options[0].Label != "Re-render again" || d.Options[1].Label != "Keep it" || !strings.Contains(d.Question, "already shortened its lines and re-rendered it once") {
		t.Fatalf("%+v", d)
	}
	if f.writes != 1 || len(f.renders) != 1 {
		t.Fatalf("writes %d renders %d", f.writes, len(f.renders))
	}
	// A second run (or a reload) finds the retry in the decision log.
	again := f.check(t, f.newRunner(true))
	if len(again) != 1 || !again[0].Retried || f.writes != 1 || len(f.renders) != 1 {
		t.Fatalf("second run: %+v writes %d renders %d", again, f.writes, len(f.renders))
	}
	// So does the chat card's automatic apply.
	newest := again[0].ID
	if problem := f.s.speechRetryProblem(f.project, f.scene.ID, newest); !strings.Contains(problem, "already shortened") {
		t.Fatalf("problem %q", problem)
	}
}

// Wrong language, a check that couldn't listen, and Manual or Auto: no retry at all.
func TestFullAutoNeverRetriesWhatItShouldNot(t *testing.T) {
	english := spoken("He", 0.6, 0.9, "came", 0.9, 1.2, "back", 1.2, 1.6, "guard", 4.0, 4.5, "the", 4.5, 4.7, "machine", 4.7, 5.4)
	for _, tc := range []struct {
		name  string
		full  bool
		heard []providers.SpokenWord
		flags int
	}{
		{name: "wrong language", full: true, heard: english, flags: 1},
		{name: "listening unavailable", full: true, heard: nil, flags: 0},
		{name: "manual or auto", full: false, heard: cutOffSpeech(), flags: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetryFixture(t, tc.full)
			f.heard = [][]providers.SpokenWord{tc.heard}
			flagged := f.check(t, f.runner)
			if len(flagged) != tc.flags || f.writes != 0 || len(f.renders) != 0 {
				t.Fatalf("flagged %+v writes %d renders %d", flagged, f.writes, len(f.renders))
			}
			if !tc.full {
				// Manual and Auto never listen by themselves; handed a flag, still nothing.
				flag := selfCheckFlag{Kind: selfCheckClipAudio, ID: f.out.ID, SceneID: f.scene.ID, Verdict: verdictFix, Issues: []selfCheckIssue{{Code: "speech_cut_off"}}}
				if left, _, _ := f.runner.retryClipSpeech(f.ctx, []selfCheckFlag{flag}); len(left) != 1 || f.writes != 0 || len(f.renders) != 0 {
					t.Fatal("a retry outside Full auto")
				}
			}
			if decisions, _ := f.s.database.ListDecisions(f.project); len(decisions) != 0 {
				t.Fatalf("decisions %+v", decisions)
			}
		})
	}
}

// The chat's automatic apply (Full auto's card) is the same one retry: it records it,
// and a click in Manual or Auto never does.
func TestAutomaticRepairApplyIsTheOneRetry(t *testing.T) {
	s, scene, out, mei := repairFixture(t)
	answer := `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住机器！","shot":1},{"id":"l3","character_id":"","text":"他回来了。","shot":1}]`
	writerSays(t, s, &answer)
	renders := fakeRepairRender(t)
	apply := func(automatic bool) int {
		repair, err := s.proposeSpeechRepair(context.Background(), *scene.ProjectID, out.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(speechRepairApply{After: repair.After, ExpectedUpdatedAt: repair.ExpectedUpdatedAt, Quote: repair.Quote, Automatic: automatic})
		return call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair/apply", string(raw)).Code
	}
	if code := apply(false); code != 200 {
		t.Fatal(code)
	}
	if decisions, _ := s.database.ListDecisions(*scene.ProjectID); len(decisions) != 0 {
		t.Fatal("a click recorded an automatic retry")
	}
	answer = `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住！","shot":1},{"id":"l3","character_id":"","text":"回来了。","shot":1}]`
	if code := apply(true); code != 200 {
		t.Fatal(code)
	}
	if _, err := s.database.GetDecision(*scene.ProjectID, speechRetryKey(out.ID)); err != nil {
		t.Fatal("the automatic retry was not recorded")
	}
	answer = `[{"id":"l2","character_id":"` + mei.ID + `","text":"守！","shot":1},{"id":"l3","character_id":"","text":"回。","shot":1}]`
	if code := apply(true); code != 409 || len(*renders) != 2 {
		t.Fatalf("a second automatic retry: %d, %d renders", code, len(*renders))
	}
}

// Full auto's quote states the worst case: each scene whose clips speak made once more.
func TestFullAutoQuoteIncludesTheSpeechRetryWorstCase(t *testing.T) {
	f := newRetryFixture(t, true)
	read := func(query string) map[string]any {
		var got map[string]any
		w := call(t, f.s, "GET", "/costs/estimate?"+query, "")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return got
	}
	plain := read("scene_ids=" + f.scene.ID)
	if _, ok := plain["worst_seconds"]; ok {
		t.Fatalf("a clicked run has no retries: %+v", plain)
	}
	full := read("full_auto=true&scene_ids=" + f.scene.ID)
	if full["retry_seconds"] != 8.0 || full["worst_seconds"] != full["seconds"].(float64)+8 || !strings.Contains(fmt.Sprint(full["retry_note"]), "once, never more") {
		t.Fatalf("worst case: %+v", full)
	}
	// Once the scene had its retry, no second one is priced.
	if err := f.s.recordSpeechRetry(f.project, f.out.ID, f.scene.Title); err != nil {
		t.Fatal(err)
	}
	if again := read("full_auto=true&scene_ids=" + f.scene.ID); again["retry_seconds"] != 0.0 {
		t.Fatalf("after the retry: %+v", again)
	}
	// At the one confirmation, before the story is written: the target seconds, twice.
	brief := read("full_auto=true&target_seconds=30&aspect_ratio=9:16")
	if brief["seconds"] != 30.0 || brief["worst_seconds"] != 60.0 {
		t.Fatalf("brief: %+v", brief)
	}
}

// In Full auto the chat asks about speech only after the retry, or when a shorter line
// can't fix it; a clip still to be repaired is acted on instead.
func TestFullAutoSpeechAsksOnlyAfterTheRetryOrForWrongLanguage(t *testing.T) {
	ask := chatDecision{Mode: decisionAsk, Question: "Re-render it?", Reason: askExpensive, Key: "selfcheck:o1", Options: []chatDecisionOption{{Label: "Re-render with a shorter line", Value: "x"}, {Label: "Keep it", Value: "y"}}}
	cut := selfCheckFlag{Kind: selfCheckClipAudio, ID: "o1", SceneID: "s1", Verdict: verdictFix, Issues: []selfCheckIssue{{Code: "speech_cut_off"}}}
	retried := cut
	retried.Retried = true
	wrong := selfCheckFlag{Kind: selfCheckClipAudio, ID: "o1", SceneID: "s1", Verdict: verdictFail, Issues: []selfCheckIssue{{Code: "wrong_language"}}}
	for _, tc := range []struct {
		name string
		flag selfCheckFlag
		asks bool
	}{{"still to be repaired", cut, false}, {"after the retry", retried, true}, {"wrong language", wrong, true}} {
		turn := &chatTurn{selfChecks: []selfCheckFlag{tc.flag}}
		got := applyFullAutoAsk(ask, "render_scene", false, turn.clipFlagged())
		if (got.Mode == decisionAsk) != tc.asks {
			t.Fatalf("%s: %+v", tc.name, got)
		}
	}
}
