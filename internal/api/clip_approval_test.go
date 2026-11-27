package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

const failClip = `{"self_check":{"kind":"clip","result":{"verdict":"fail","score":12,"issues":[{"code":"blank_frame","severity":"high","detail":"The middle frame is black."}]},"hash":"x"}}`
const passClip = `{"self_check":{"kind":"clip","result":{"verdict":"pass","score":90,"issues":[]},"hash":"x"}}`

// approvalScene is a scene with one finished clip whose notes are qa.
func approvalScene(t *testing.T, s *Server, project, title string, order int, qa string, script *string) (*models.Scene, *models.RenderOutput) {
	t.Helper()
	scene := &models.Scene{ProjectID: &project, ScriptID: script, Title: title, Summary: title, SceneOrder: order, Duration: 4, AspectRatio: "9:16", CharacterIDsJSON: json.RawMessage(`[]`), SceneJSON: json.RawMessage(`{}`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	if qa == "" {
		return scene, nil
	}
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: models.StatusSucceeded, RequestJSON: json.RawMessage(`{"render_plan_id":"p-` + title + `","render_unit":{"key":"p-` + title + `:0","index":0,"count":1}}`)}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	video := "storage/outputs/" + job.ID + ".mp4"
	full, _ := s.storagePath(video)
	os.MkdirAll(strings.TrimSuffix(full, job.ID+".mp4"), 0700)
	os.WriteFile(full, []byte("clip "+job.ID), 0600)
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: video, QAJSON: json.RawMessage(qa)}
	if err := s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	return scene, out
}

func TestApproveAllApprovesOnlyFinishedPassedClips(t *testing.T) {
	s, project := selfCheckServer(t)
	_, passed := approvalScene(t, s, project, "Dawn", 1, passClip, nil)
	_, flagged := approvalScene(t, s, project, "Noon", 2, failClip, nil)
	_, unchecked := approvalScene(t, s, project, "Dusk", 3, `{}`, nil)
	approvalScene(t, s, project, "Night", 4, "", nil) // no clip yet

	w := call(t, s, "POST", "/clips/approve-all", `{}`)
	var got struct {
		Approved  []clipApprovalEntry `json:"approved"`
		Already   []clipApprovalEntry `json:"already_approved"`
		Skipped   []clipApprovalEntry `json:"skipped"`
		Unchecked []clipApprovalEntry `json:"unchecked"`
		Message   string              `json:"message"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(got.Approved) != 2 || got.Approved[0].OutputID != passed.ID || got.Approved[1].OutputID != unchecked.ID {
		t.Fatalf("approved %+v", got.Approved)
	}
	if len(got.Unchecked) != 1 || got.Unchecked[0].OutputID != unchecked.ID {
		t.Fatalf("unchecked %+v", got.Unchecked)
	}
	reasons := map[string]string{}
	for _, e := range got.Skipped {
		reasons[e.OutputID+e.Label] = e.Reason
	}
	if len(got.Skipped) != 2 || !strings.Contains(reasons[flagged.ID+"“Noon”"], "middle frame is black") || reasons["“Night”"] != "no finished clip yet" {
		t.Fatalf("skipped %+v", got.Skipped)
	}
	if !strings.Contains(got.Message, "Approved 2 clips") || !strings.Contains(got.Message, "“Noon”") {
		t.Fatal(got.Message)
	}
	for id, want := range map[string]bool{passed.ID: true, unchecked.ID: true, flagged.ID: false} {
		o, _ := s.database.GetRenderOutput(id)
		if clipApproved(o.QAJSON) != want || (want && !strings.Contains(string(o.QAJSON), `"by":"user"`)) {
			t.Fatalf("%s approved=%v: %s", id, !want, o.QAJSON)
		}
	}
	// The approval is a note: the clip's check is kept.
	if o, _ := s.database.GetRenderOutput(passed.ID); storedSelfCheck(o.QAJSON) == nil {
		t.Fatal("approval erased the check")
	}
	// Again: nothing new, the approved ones are reported as such.
	w = call(t, s, "POST", "/clips/approve-all", `{}`)
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Approved) != 0 || len(got.Already) != 2 {
		t.Fatal(w.Body.String())
	}
	// Keeping the flagged clip (the check's Keep it) lets it be approved.
	if a := call(t, s, "POST", "/selfcheck/accept", fmt.Sprintf(`{"kind":"clip","id":%q}`, flagged.ID)); a.Code != 200 {
		t.Fatal(a.Body.String())
	}
	w = call(t, s, "POST", "/clips/approve-all", fmt.Sprintf(`{"output_ids":[%q]}`, flagged.ID))
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Approved) != 1 || got.Approved[0].OutputID != flagged.ID {
		t.Fatal(w.Body.String())
	}
}

func TestApproveAllScopesToTheStory(t *testing.T) {
	s, project := selfCheckServer(t)
	script := &models.Script{ProjectID: &project, Idea: "x", Title: "X", Summary: "x", DraftJSON: []byte("{}")}
	s.database.CreateScript(script)
	_, inStory := approvalScene(t, s, project, "In", 1, passClip, &script.ID)
	_, other := approvalScene(t, s, project, "Out", 2, passClip, nil)
	report, err := s.approveClips(project, approveClipsRequest{ScriptID: script.ID}, approvalByFullAuto)
	if err != nil || len(report.Approved) != 1 || report.Approved[0].OutputID != inStory.ID {
		t.Fatal(err, report)
	}
	if o, _ := s.database.GetRenderOutput(other.ID); clipApproved(o.QAJSON) {
		t.Fatal("a clip of another story was approved")
	}
	if o, _ := s.database.GetRenderOutput(inStory.ID); !strings.Contains(string(o.QAJSON), `"by":"full_auto"`) {
		t.Fatal(string(o.QAJSON))
	}
}

func TestFinalRenderRefusesUnapprovedClips(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	s, project := selfCheckServer(t)
	_, out := approvalScene(t, s, project, "Dawn", 1, passClip, nil)
	full, _ := s.storagePath(out.VideoPath)
	if b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=90x160:r=24", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", full).CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	body := fmt.Sprintf(`{"version":1,"width":90,"height":160,"fps":24,"duration":2,"clips":[{"id":"c1","asset_id":%q,"start":0,"trim":0,"duration":2,"speed":1}],"audio":[],"captions":[]}`, "output-"+out.ID)
	saved := call(t, s, "POST", "/compositions", body)
	var m struct {
		Revision string `json:"revision"`
	}
	if saved.Code != 201 || json.Unmarshal(saved.Body.Bytes(), &m) != nil {
		t.Fatal(saved.Code, saved.Body.String())
	}
	run := func(op string) (int, map[string]any) {
		w := call(t, s, "POST", "/compositions/"+m.Revision+"/run", fmt.Sprintf(`{"revision":%q,"operation":%q}`, m.Revision, op))
		var got map[string]any
		json.Unmarshal(w.Body.Bytes(), &got)
		return w.Code, got
	}
	code, got := run("render")
	if code != http.StatusConflict || got["code"] != "clips_not_approved" || !strings.Contains(fmt.Sprint(got["error"]), "“Dawn” is not approved") {
		t.Fatal(code, got)
	}
	if list, _ := got["unapproved"].([]any); len(list) != 1 || list[0].(map[string]any)["output_id"] != out.ID {
		t.Fatal(got)
	}
	// Previews are not the final video and are never held back.
	if code, got = run("lint"); got["code"] == "clips_not_approved" {
		t.Fatal("lint refused", code)
	}
	if w := call(t, s, "POST", "/clips/approve-all", fmt.Sprintf(`{"output_ids":[%q]}`, out.ID)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if code, got = run("render"); got["code"] == "clips_not_approved" {
		t.Fatal("still refused after approval", code, got)
	}
}

// keyframeServer serves only the free model lists: a first-frame video model.
func keyframeServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[4],"supported_aspect_ratios":["9:16"],"supported_frame_images":["first_frame"]}]}`))
		case "/images/models":
			w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("a keyframe test must never call a paid service: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Keyframes")
	return s, project
}

func TestFullAutoMakesEachKeyframeOnceInStoryOrderContinuingThePrevious(t *testing.T) {
	s, project := keyframeServer(t)
	scenes := []models.Scene{}
	for i, title := range []string{"Arrival", "Chase", "Escape"} {
		scene := &models.Scene{ProjectID: &project, Title: title, Summary: title, SceneOrder: i + 1, Duration: 4, AspectRatio: "9:16", CharacterIDsJSON: json.RawMessage(`[]`), SceneJSON: json.RawMessage(`{}`)}
		s.database.CreateScene(scene)
		s.database.CreateShot(&models.Shot{SceneID: scene.ID, ShotOrder: 1, Duration: 4, Prompt: title + " shot"})
		scene, _ = s.database.GetScene(scene.ID)
		scenes = append(scenes, *scene)
	}
	type made struct{ scene, frame, note string }
	calls := []made{}
	old := autopilotKeyframeMaker
	t.Cleanup(func() { autopilotKeyframeMaker = old })
	autopilotKeyframeMaker = func(s *Server, _ context.Context, scene *models.Scene, frame, note string) (string, error) {
		calls = append(calls, made{scene.Title, frame, note})
		asset := &models.Asset{ProjectID: &project, Type: "storyboard", Name: scene.Title + " storyboard", FilePath: writePicture(t, s, "board-"+scene.ID, 18, 32, 90), TagsJSON: json.RawMessage(`[]`), MetadataJSON: json.RawMessage(`{"scene_id":"` + scene.ID + `","consistency_review":"ready"}`)}
		if err := s.database.CreateAsset(asset); err != nil {
			return "", err
		}
		current, _ := s.database.GetScene(scene.ID)
		_, err := s.attachStoryboard(current, []models.Asset{*asset})
		return asset.ID, err
	}
	runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{SceneIDs: []string{scenes[0].ID, scenes[1].ID, scenes[2].ID}, VideosOnly: true, FullAuto: true}, scenes: scenes}
	ctx := checkContext(t, s, project)
	if err := runner.makeKeyframes(ctx, map[string][]string{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].scene != "Arrival" || calls[1].scene != "Chase" || calls[2].scene != "Escape" {
		t.Fatalf("%+v", calls)
	}
	if calls[0].frame != "" || calls[0].note != "" {
		t.Fatal("the first scene has nothing to continue from", calls[0])
	}
	for _, c := range calls[1:] {
		if !strings.HasPrefix(c.frame, "data:image/png;base64,") || !strings.Contains(c.note, "Keep the same characters") || !strings.Contains(c.note, "opening keyframe") {
			t.Fatalf("no continuity for %s: %q", c.scene, c.note)
		}
	}
	if !strings.Contains(calls[2].note, "“Chase”") {
		t.Fatal(calls[2].note)
	}
	if runner.notes.keyframesMade != 3 || !strings.Contains(runner.summary(false), "ade 3 keyframes") {
		t.Fatal(runner.summary(false))
	}
	// Run again: every keyframe is current, so none is paid for twice.
	runner.notes = autopilotNotes{}
	for i := range runner.scenes {
		fresh, _ := s.database.GetScene(runner.scenes[i].ID)
		runner.scenes[i] = *fresh
	}
	if err := runner.makeKeyframes(ctx, map[string][]string{}); err != nil || len(calls) != 3 || runner.notes.keyframesReused != 3 {
		t.Fatal(err, len(calls), runner.notes)
	}
	// Not Full auto: the render makes its own keyframe as before.
	runner.body.FullAuto = false
	if err := runner.makeKeyframes(ctx, nil); err != nil || len(calls) != 3 {
		t.Fatal("only Full auto makes keyframes ahead")
	}
}

func TestContinuityUsesThePreviousClipsLastFrame(t *testing.T) {
	s, project := selfCheckServer(t)
	defer fakeFrames(t, s)()
	scene, _ := approvalScene(t, s, project, "Before", 1, passClip, nil)
	frame, fromClip := s.previousScenePicture(context.Background(), scene)
	if !fromClip || !strings.HasPrefix(frame, "data:image/png;base64,") {
		t.Fatal(fromClip, len(frame))
	}
	if note := continuityNote("Before", true); !strings.Contains(note, "last frame of its clip") {
		t.Fatal(note)
	}
	// The previous picture is one more reference, within the services' limit of 8.
	ctx := context.WithValue(context.Background(), continuityFrameKey{}, "data:x")
	refs := withContinuityFrame(ctx, []string{"1", "2", "3", "4", "5", "6", "7", "8"})
	if len(refs) != 8 || refs[7] != "data:x" {
		t.Fatal(refs)
	}
	if refs = withContinuityFrame(context.Background(), []string{"1"}); len(refs) != 1 {
		t.Fatal(refs)
	}
}

func TestAutopilotApprovesPassedClipsAndReportsTheRest(t *testing.T) {
	s, project := selfCheckServer(t)
	a, passed := approvalScene(t, s, project, "Dawn", 1, passClip, nil)
	b, flagged := approvalScene(t, s, project, "Noon", 2, failClip, nil)
	runner := &autopilotRunner{s: s, op: &models.Op{ID: "op"}, project: project, body: autopilotRequest{SceneIDs: []string{a.ID, b.ID}, VideosOnly: true, FullAuto: true}, scenes: []models.Scene{*a, *b}}
	report, err := runner.approveRunClips()
	if err != nil || len(report.Approved) != 1 || report.Approved[0].OutputID != passed.ID || len(report.Skipped) != 1 || report.Skipped[0].OutputID != flagged.ID {
		t.Fatal(err, report)
	}
	if sum := runner.summary(false); !strings.Contains(sum, "approved 1 clip,") || !strings.Contains(sum, "left 1 for you to review") || !strings.Contains(sum, "checked every picture and clip") {
		t.Fatal(sum)
	}
	if sum := runner.summary(true); !strings.Contains(sum, "exported the final video") {
		t.Fatal(sum)
	}
}

func TestApproveClipsIsAChatAction(t *testing.T) {
	if canonicalChatAction(approveClipsAction) != approveClipsAction {
		t.Fatal("approve_clips is not a chat action")
	}
	for _, kind := range []string{"scene", "character", "output", ""} {
		if !selectedChatActionAllowed(kind, approveClipsAction) {
			t.Fatalf("approve_clips must work whatever is selected (%q)", kind)
		}
	}
}

func TestApproveAllOnTheUsersWordApprovesFlaggedClipsToo(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	story := "story-1"
	_, flagged := approvalScene(t, s, project, "Noon", 1, failClip, &story)
	approvalScene(t, s, project, "Night", 2, "", &story)
	w := call(t, s, "POST", "/clips/approve-all", `{"include_flagged":true}`)
	var got struct {
		Approved   []clipApprovalEntry `json:"approved"`
		Overridden []clipApprovalEntry `json:"overridden"`
		Skipped    []clipApprovalEntry `json:"skipped"`
		ScriptID   string              `json:"script_id"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(got.Overridden) != 1 || got.Overridden[0].OutputID != flagged.ID || got.ScriptID != story {
		t.Fatal(w.Body.String())
	}
	out, _ := s.database.GetRenderOutput(flagged.ID)
	decisions, _ := s.database.ListDecisions(project)
	if !clipApproved(out.QAJSON) || !clipKeptInLog(decisions, flagged.ID) {
		t.Fatal("the flagged clip must be approved and its finding settled")
	}
}
