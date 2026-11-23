package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

func TestSelfCheckPolicyRetriesPicturesOnceAndNeverClips(t *testing.T) {
	record := func(verdict string, attempt int, worth bool) selfCheckRecord {
		return selfCheckRecord{Attempt: attempt, Result: selfCheckResult{Verdict: verdict, Issues: []selfCheckIssue{{Code: "x", Severity: "high", Detail: "d"}}, Retry: selfCheckRetry{WorthIt: worth, Change: "c"}}}
	}
	accepted := record(verdictFail, 1, true)
	now := time.Now()
	accepted.AcceptedAt = &now
	unavailable := selfCheckRecord{Attempt: 1, Result: unavailableResult("no session")}
	for _, tc := range []struct {
		name     string
		full     bool
		r        selfCheckRecord
		want     string
		clipWant string
	}{
		{"pass", true, record(verdictPass, 1, false), selfCheckKeep, selfCheckKeep},
		{"unavailable never blocks", true, unavailable, selfCheckKeep, selfCheckKeep},
		{"kept by the user", true, accepted, selfCheckKeep, selfCheckKeep},
		{"first failure worth retrying", true, record(verdictFail, 1, true), selfCheckRemake, selfCheckAsk},
		{"fix worth retrying", true, record(verdictFix, 1, true), selfCheckRemake, selfCheckAsk},
		{"retry already made", true, record(verdictFix, 2, true), selfCheckStop, selfCheckAsk},
		{"not worth retrying", true, record(verdictFail, 1, false), selfCheckStop, selfCheckAsk},
		{"manual shows findings only", false, record(verdictFail, 1, true), selfCheckShow, selfCheckShow},
	} {
		if got := pictureSelfCheckStep(tc.full, tc.r); got != tc.want {
			t.Errorf("%s: picture step %s, want %s", tc.name, got, tc.want)
		}
		if got := clipSelfCheckStep(tc.full, tc.r, false); got != tc.clipWant {
			t.Errorf("%s: clip step %s, want %s", tc.name, got, tc.clipWant)
		}
	}
	if clipSelfCheckStep(true, record(verdictFail, 1, true), true) != selfCheckKeep {
		t.Error("a clip kept in the chat must not be asked about again")
	}
}

func TestFullAutoAllowsTheFlaggedClipQuestionOnly(t *testing.T) {
	flag := selfCheckFlag{Kind: selfCheckClip, ID: "out-1", Label: "“Chase” clip", Verdict: verdictFix, Issues: []selfCheckIssue{{Code: "missing_character", Severity: "high", Detail: "Mia is missing."}}, Change: "Show Mia running."}
	d := clipReviewDecision(flag)
	if d.Mode != decisionAsk || d.Reason != askExpensive || d.Key != "selfcheck:out-1" || len(d.Options) != 2 || d.Options[0].Label != "Re-render with this change" || d.Options[1].Label != "Keep it" || !strings.Contains(d.Options[0].Value, "Show Mia running.") || !validDecisionKey(d.Key) {
		t.Fatalf("%+v", d)
	}
	if got := applyFullAutoAsk(d, "retry_render", false, true); got.Mode != decisionAsk {
		t.Fatalf("a flagged clip's question must stand in Full auto: %+v", got)
	}
	if got := applyFullAutoAsk(d, "render_scene", false, false); got.Mode != decisionAct || got.Overruled != "full_auto" {
		t.Fatalf("with no flagged clip the expensive question is overruled: %+v", got)
	}
	other := chatDecision{Mode: decisionAsk, Question: "Which music?", Reason: askExpensive, Options: []chatDecisionOption{{Label: "Piano", Value: "Piano"}}}
	if got := applyFullAutoAsk(other, "finish_video", false, true); got.Mode != decisionAct {
		t.Fatalf("an unrelated expensive question stays overruled: %+v", got)
	}
	// "Keep it" picked as a quick reply is recorded by recordAnswer and keeps the clip.
	kept := []db.ProjectDecision{{Key: "selfcheck:out-1", Value: "Keep it — Keep the “Chase” clip as it is."}}
	if !clipKeptInLog(kept, "out-1") || clipKeptInLog([]db.ProjectDecision{{Key: "selfcheck:out-1", Value: "Re-render with this change — …"}}, "out-1") {
		t.Fatal("only Keep it keeps the clip")
	}
}

func TestReviewStageReflectsSelfCheck(t *testing.T) {
	scene := stageScene{ID: "s1", Title: "Chase", Progress: sceneProgress{Stage: "video_ready", Video: sceneVideo{State: "ready"}}}
	in := stageInputs{BriefDecided: true, Scenes: []stageScene{scene}, LastClipAt: time.Now()}
	if st := reviewStage(in); st.Status != statusReady || len(st.Findings) != 0 {
		t.Fatalf("no check yet keeps the old review stage: %+v", st)
	}
	failed := &selfCheckRecord{Kind: selfCheckClip, ID: "o1", Result: selfCheckResult{Verdict: verdictFail, Score: 20, Issues: []selfCheckIssue{{Code: "blank_frame", Severity: "high", Detail: "The last frame is black."}}, Retry: selfCheckRetry{WorthIt: true, Change: "No fade to black."}}}
	in.Clips = []stageClip{{SceneID: "s1", SceneTitle: "Chase", OutputID: "o1", Check: failed}}
	st := reviewStage(in)
	if st.Status != statusBlocked || len(st.Findings) != 1 || st.Findings[0].Verdict != verdictFail || len(st.Missing) != 1 || !strings.Contains(st.Missing[0], "The last frame is black") || st.Next == nil || st.Next.Cost.Class != costFree || st.Total != 1 || st.Done != 0 {
		t.Fatalf("%+v", st)
	}
	in.Reviewed = true // watching the clips does not clear a flagged clip
	if reviewStage(in).Status != statusBlocked {
		t.Fatal("a flagged clip stays flagged until it is kept or made again")
	}
	in.Clips[0].Kept = true
	if st := reviewStage(in); st.Status != statusDone || !st.Findings[0].Kept || st.Done != 1 {
		t.Fatalf("%+v", st)
	}
	in.Reviewed = false
	in.Clips = []stageClip{{SceneID: "s1", SceneTitle: "Chase", OutputID: "o2", Check: &selfCheckRecord{Result: selfCheckResult{Verdict: verdictPass, Score: 90}}}}
	if st := reviewStage(in); st.Status != statusDone {
		t.Fatalf("every clip passing is a reviewed project: %+v", st)
	}
	// The blocked review stage is optional: the project's current stage moves on to Edit.
	in.Clips = []stageClip{{SceneID: "s1", SceneTitle: "Chase", OutputID: "o1", Check: failed}}
	m := computeStages(in)
	if m.Current != stageEdit {
		t.Fatalf("%+v", m.Current)
	}
	if lines := chatStages(m)["stages"].([]string); !strings.Contains(strings.Join(lines, "\n"), "review: blocked") {
		t.Fatal(lines)
	}
}

func TestCurrentSceneClipJobsPicksTheNewestCompleteRender(t *testing.T) {
	scene := "s1"
	at := time.Now()
	job := func(id, plan string, index, count int, status string, age time.Duration) models.RenderJob {
		raw, _ := json.Marshal(map[string]any{"render_plan_id": plan, "render_unit": map[string]any{"key": plan + ":" + fmt.Sprint(index), "index": index, "count": count}})
		return models.RenderJob{ID: id, SceneID: &scene, Status: status, RequestJSON: raw, UpdatedAt: at.Add(-age)}
	}
	jobs := []models.RenderJob{
		job("old0", "old", 0, 1, models.StatusSucceeded, time.Hour),
		job("new0", "new", 0, 2, models.StatusSucceeded, time.Minute),
		job("new1", "new", 1, 2, models.StatusSucceeded, 2*time.Minute),
		job("half0", "half", 0, 2, models.StatusSucceeded, 0), // incomplete: never current
		job("fail", "x", 0, 1, models.StatusFailed, 0),
	}
	got := currentSceneClipJobs(jobs)["s1"]
	if len(got) != 2 || got[0].ID != "new0" || got[1].ID != "new1" {
		t.Fatalf("%+v", got)
	}
}

func TestFullAutoRetriesAFlaggedPictureOnceThenStops(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []string
		remakes int
		checks  int
		stops   bool
	}{
		{"fixed by the retry", []string{extraCharAnswer, passAnswer}, 1, 2, false},
		{"still flagged after the retry", []string{extraCharAnswer, extraCharAnswer}, 1, 2, true},
		{"not worth retrying", []string{`{"verdict":"fail","score":10,"issues":[{"code":"distorted_anatomy","severity":"high","detail":"Six fingers."}],"suggestion":"","retry":{"worth_it":false,"change":""}}`}, 0, 1, true},
		{"passes", []string{passAnswer}, 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, project := selfCheckServer(t)
			calls, _ := selfCheckCLI(t, s, map[string][]string{"character_sheet": tc.answers})
			c, _ := sheetFixture(t, s, project)
			scene, _ := clipFixture(t, s, project, c)
			remakes := 0
			old := selfCheckRemakePicture
			selfCheckRemakePicture = func(s *Server, ctx context.Context, target *selfCheckTarget, change string) (*selfCheckTarget, error) {
				remakes++
				if change != "Mia alone, no other people." {
					t.Errorf("retry without the reviewer's change: %q", change)
				}
				// A new picture, bound to the character in place of the old one.
				asset := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: "Mia reference", FilePath: writePicture(t, s, fmt.Sprintf("mia-retry-%d", remakes), 64, 32, 200), TagsJSON: json.RawMessage(`[]`), MetadataJSON: json.RawMessage(`{"consistency_review":"ready"}`)}
				s.database.CreateAsset(asset)
				c.ReferenceAssetIDsJSON, _ = json.Marshal([]string{asset.ID})
				s.database.UpdateCharacter(c)
				return s.characterSheetTarget(project, asset.ID)
			}
			defer func() { selfCheckRemakePicture = old }()
			runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{SceneIDs: []string{scene.ID}, VideosOnly: true, FullAuto: true}, scenes: []models.Scene{*scene}}
			ctx := checkContext(t, s, project)
			result, err := runner.checkPictures(ctx)
			if remakes != tc.remakes || calls("character_sheet") != tc.checks || (err != nil) != tc.stops {
				t.Fatalf("remakes %d checks %d err %v", remakes, calls("character_sheet"), err)
			}
			if !tc.stops {
				return
			}
			flagged := result.(map[string]any)["self_check"].([]selfCheckFlag)
			if len(flagged) != 1 || !strings.Contains(err.Error(), "No video was made") {
				t.Fatal(result, err)
			}
			// Running again never loops: nothing changed, so no new turn and no new picture.
			if _, err = runner.checkPictures(ctx); err == nil || remakes != tc.remakes || calls("character_sheet") != tc.checks {
				t.Fatalf("second run: remakes %d checks %d err %v", remakes, calls("character_sheet"), err)
			}
			// Keeping it lets Full auto carry on.
			kind, id := flagged[0].Kind, flagged[0].ID
			if a := call(t, s, "POST", "/selfcheck/accept", fmt.Sprintf(`{"kind":%q,"id":%q}`, kind, id)); a.Code != 200 {
				t.Fatal(a.Body.String())
			}
			if _, err = runner.checkPictures(ctx); err != nil || calls("character_sheet") != tc.checks {
				t.Fatal(err)
			}
		})
	}
}

func TestFullAutoNeverRemakesAClipAndAsksInstead(t *testing.T) {
	s, project := selfCheckServer(t)
	defer fakeFrames(t, s)()
	calls, _ := selfCheckCLI(t, s, map[string][]string{"clip": {`{"verdict":"fail","score":15,"issues":[{"code":"blank_frame","severity":"high","detail":"The middle frame is black."}],"suggestion":"","retry":{"worth_it":true,"change":"No black frames."}}`}})
	c, _ := sheetFixture(t, s, project)
	scene, out := clipFixture(t, s, project, c)
	jobsBefore, _ := s.database.ListRenderJobs(project)
	remade := false
	old := selfCheckRemakePicture
	selfCheckRemakePicture = func(*Server, context.Context, *selfCheckTarget, string) (*selfCheckTarget, error) {
		remade = true
		return nil, fmt.Errorf("no")
	}
	defer func() { selfCheckRemakePicture = old }()
	runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{SceneIDs: []string{scene.ID}, VideosOnly: true, FullAuto: true}, scenes: []models.Scene{*scene}}
	ctx := checkContext(t, s, project)
	flagged := runner.checkClips(ctx)
	jobsAfter, _ := s.database.ListRenderJobs(project)
	if len(flagged) != 1 || flagged[0].ID != out.ID || remade || len(jobsAfter) != len(jobsBefore) || calls("clip") != 1 {
		t.Fatalf("flagged %+v remade %v jobs %d→%d", flagged, remade, len(jobsBefore), len(jobsAfter))
	}
	result := runner.clipReviewResult(flagged)
	d := result["decision"].(chatDecision)
	if result["needs_review"] != true || d.Mode != decisionAsk || d.Reason != askExpensive || d.Key != selfCheckDecisionKey(out.ID) {
		t.Fatalf("%+v", result)
	}
	// Not Full auto: nothing is checked by the run.
	runner.body.FullAuto = false
	if len(runner.checkClips(ctx)) != 0 || calls("clip") != 1 {
		t.Fatal("only Full auto checks clips by itself")
	}
	// The user keeps it in the chat: the run no longer flags it, without a new turn.
	runner.body.FullAuto = true
	s.recordAnswer(project, &chatAnswer{Key: d.Key, Question: d.Question, Label: "Keep it", Value: d.Options[1].Value})
	if len(runner.checkClips(ctx)) != 0 || calls("clip") != 1 {
		t.Fatal("a kept clip was flagged or checked again")
	}
}

func TestFlaggedClipReachesStagesAndChat(t *testing.T) {
	s, project := selfCheckServer(t)
	defer fakeFrames(t, s)()
	selfCheckCLI(t, s, map[string][]string{"clip": {`{"verdict":"fix","score":48,"issues":[{"code":"missing_character","severity":"high","detail":"Mia is not in the frames."}],"suggestion":"","retry":{"worth_it":true,"change":"Keep Mia in frame the whole time."}}`}})
	c, _ := sheetFixture(t, s, project)
	_, out := clipFixture(t, s, project, c)
	if w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip","id":%q}`, out.ID)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	// The stage map shows it, and the chat context carries it with its question key.
	m, err := s.projectStages(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range m.Stages {
		if st.ID == stageReview && (len(st.Findings) != 1 || st.Findings[0].OutputID != out.ID) {
			t.Fatalf("%+v", st)
		}
	}
	progress, _ := s.projectProgress(context.Background(), project)
	in, _ := s.stageInputs(context.Background(), project, progress)
	ctx := chatSelfCheck(flaggedClips(in.Clips))
	if items := ctx["flagged"].([]map[string]any); len(items) != 1 || items[0]["decision_key"] != "selfcheck:"+out.ID || items[0]["change"] != "Keep Mia in frame the whole time." {
		t.Fatalf("%v", items)
	}
}

func TestChatTurnKeepsTheFlaggedClipQuestionInFullAuto(t *testing.T) {
	res := &agents.ChatIntentResponse{Decision: &agents.ChatDecision{Mode: "ask", Question: "Re-render the chase clip?", Reason: "expensive", Key: "selfcheck:out-1", Options: []agents.DecisionOption{{Label: "Re-render with this change", Value: "Re-render it"}, {Label: "Keep it", Value: "Keep it"}}}}
	turn := &chatTurn{body: chatRequest{Full: true, Message: "how did it go?"}, selfChecks: []selfCheckFlag{{Kind: selfCheckClip, ID: "out-1"}}}
	if d := chatTurnDecision(turn, res, "retry_render"); d.Mode != decisionAsk || d.Key != "selfcheck:out-1" {
		t.Fatalf("%+v", d)
	}
	turn.selfChecks = []selfCheckFlag{{Kind: selfCheckCharacterSheet, ID: "a1"}}
	if d := chatTurnDecision(turn, res, "retry_render"); d.Mode != decisionAct || d.Overruled != "full_auto" {
		t.Fatalf("a flagged picture is no reason to ask about video credits: %+v", d)
	}
	turn.body.Full = false
	if d := chatTurnDecision(turn, res, "retry_render"); d.Mode != decisionAsk {
		t.Fatalf("Manual and Auto keep their questions: %+v", d)
	}
}
