package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// Repair a clip's speech: the speech check (audio_check.go) heard a clip cut off a
// line, say different words or leave a line out. The local AI session rewrites ONLY
// that clip's spoken lines, shorter and simpler, in one turn (the Fit script rewrite
// with the check's findings and a tighter target: 60-75 % of each shot's seconds),
// the lines are saved with a revision, and the scene's clip is made again. In Manual
// and Auto the before and after lines and the price of the new clip are shown and
// nothing happens until the owner clicks; Full auto does it by itself once per clip
// (speech_retry.go).

const speechRepairAction = "repair_clip_speech"

// speechRepairCodes are the findings a shorter, simpler line can fix.
var speechRepairCodes = []string{"speech_cut_off", "different_words", "line_missing"}

// speechRepairProblem says why a clip's speech check cannot be repaired by rewriting
// its lines, or "" when it can.
func speechRepairProblem(r *selfCheckRecord) string {
	switch {
	case r == nil:
		return "this clip's speech hasn't been checked yet; press Check on its Speech check first"
	case r.Result.Unavailable:
		return "the speech check couldn't listen to this clip, so there is nothing to repair"
	case r.passes():
		return "the speech check found nothing to fix in this clip"
	case hasIssue(r.Result.Issues, "wrong_language"):
		return "the clip spoke the wrong language; a shorter line won't fix that, so re-render it with the check's change or keep it"
	case hasIssue(r.Result.Issues, "no_speech"):
		return "no speech was heard in the clip; a shorter line won't fix that, so re-render it with the check's change or keep it"
	}
	for _, code := range speechRepairCodes {
		if hasIssue(r.Result.Issues, code) {
			return ""
		}
	}
	return "the speech check found nothing a shorter line would fix"
}

// speechRepairQuote is what making the scene's clip again costs, from its production
// plan with the rewritten lines: what the render button quotes.
type speechRepairQuote struct {
	PlanID      string `json:"plan_id"`
	Calls       int    `json:"calls"`
	Seconds     int    `json:"seconds"`
	Storyboards int    `json:"storyboards"`
	Blocked     string `json:"blocked,omitempty"`
}

// speechRepair is a proposal: the scene's lines before and after (only the clip's lines
// differ), what the check heard, and the price of the new clip. Nothing is saved.
type speechRepair struct {
	OutputID   string `json:"output_id"`
	SceneID    string `json:"scene_id"`
	SceneTitle string `json:"scene_title"`
	// ExpectedUpdatedAt is the scene version the proposal was made from.
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	// Heard is the speech check's findings in words; SpeechEnd and ClipLength when the
	// speech stopped and how long the clip is.
	Heard      []string          `json:"heard"`
	SpeechEnd  *float64          `json:"speech_end,omitempty"`
	ClipLength float64           `json:"clip_length"`
	LineIDs    []string          `json:"line_ids"`
	Before     []SpokenLine      `json:"before"`
	After      []SpokenLine      `json:"after"`
	BeforeFit  sceneSpeech       `json:"before_fit"`
	AfterFit   sceneSpeech       `json:"after_fit"`
	Quote      speechRepairQuote `json:"quote"`
}

// speechRepairClip is a flagged clip resolved for a repair: its check, its scene, the
// lines it says and the shots it is made of.
type speechRepairClip struct {
	OutputID string
	Record   *selfCheckRecord
	Scene    *models.Scene
	Before   []SpokenLine
	LineIDs  []string
	Shots    []models.Shot // the scene's shots, in order
	Clip     []models.Shot // the shots this clip is made of
	Language string
}

// speechRepairTarget resolves a clip the speech check flagged for a repair.
func (s *Server) speechRepairTarget(project, outputID string) (*speechRepairClip, error) {
	out, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		return nil, fmt.Errorf("video not found")
	}
	record := storedAudioCheck(out.QAJSON)
	if problem := speechRepairProblem(record); problem != "" {
		return nil, fmt.Errorf("%s", problem)
	}
	script, err := s.outputScript(project, outputID)
	if err != nil {
		return nil, err
	}
	c := &speechRepairClip{OutputID: outputID, Record: record, Scene: script.Scene, Before: sceneSpokenLines(script.Scene), Language: sceneBrief(script.Scene).Language}
	for _, l := range script.Lines {
		if l.ID != "" {
			c.LineIDs = append(c.LineIDs, l.ID)
		}
	}
	if len(c.LineIDs) == 0 {
		return nil, fmt.Errorf("“%s” has no spoken lines in this clip to shorten", c.Scene.Title)
	}
	if c.Language == "" {
		c.Language = c.Before[0].Language
	}
	if c.Shots, err = s.database.ListShots(c.Scene.ID); err != nil {
		return nil, err
	}
	sort.SliceStable(c.Shots, func(i, j int) bool { return c.Shots[i].ShotOrder < c.Shots[j].ShotOrder })
	ids := renderRequestUnit(script.Job.RequestJSON).ShotIDs
	if script.Job.ShotID != nil {
		ids = []string{*script.Job.ShotID}
	}
	in := map[string]bool{}
	for _, id := range ids {
		in[id] = true
	}
	for _, sh := range c.Shots {
		if len(ids) == 0 || in[sh.ID] {
			c.Clip = append(c.Clip, sh)
		}
	}
	return c, nil
}

// heardFindings are the speech check's findings as the writer is given them.
func heardFindings(r *selfCheckRecord) []string {
	heard := []string{}
	for _, issue := range r.Result.Issues {
		heard = append(heard, issue.Detail)
	}
	if a := r.Result.Audio; a != nil && a.SpeechEnd != nil && a.ClipLength > 0 {
		heard = append(heard, fmt.Sprintf("The clip is %.1f s long; the speech ended at %.1f s.", a.ClipLength, *a.SpeechEnd))
	}
	if change := strings.TrimSpace(r.Result.Retry.Change); change != "" {
		heard = append(heard, "Suggested: "+change)
	}
	return heard
}

// proposeSpeechRepair asks the local AI session once for the clip's lines shorter and
// simpler, checks them, and returns them beside the lines as they are, with the price
// of making the clip again. Nothing is saved.
func (s *Server) proposeSpeechRepair(ctx context.Context, project, outputID string) (*speechRepair, error) {
	c, err := s.speechRepairTarget(project, outputID)
	if err != nil {
		return nil, err
	}
	scene, cal := c.Scene, s.speechCalibration()
	names := s.speakerNames(project)
	number := map[string]int{}
	fitShots := []agents.FitShot{}
	add := func(n int, seconds float64, action string) {
		b := speech.BudgetShare(seconds, c.Language, cal, speech.RepairTarget, speech.RepairMost)
		fitShots = append(fitShots, agents.FitShot{Number: n, Seconds: int(seconds), Action: action, Target: fmt.Sprintf("%d-%d %s", b.Low, b.High, b.Unit), Most: fmt.Sprintf("%d %s", b.Max, b.Unit)})
	}
	for i, sh := range c.Clip {
		number[sh.ID] = i + 1
		add(i+1, float64(sh.Duration), sh.Prompt)
	}
	if len(c.Clip) == 0 {
		add(1, float64(scene.Duration), scene.Summary)
	}
	inClip := map[string]bool{}
	for _, id := range c.LineIDs {
		inClip[id] = true
	}
	speakers, fitLines := map[string]string{}, []agents.FitLine{}
	for _, l := range c.Before {
		if !inClip[l.ID] {
			continue
		}
		who := names[l.CharacterID]
		if l.CharacterID == "" || who == "" {
			who = "Narrator"
		}
		speakers[l.CharacterID] = who
		shot := number[l.ShotID]
		if shot == 0 {
			shot = 1
		}
		fitLines = append(fitLines, agents.FitLine{ID: l.ID, CharacterID: l.CharacterID, Speaker: who, Text: l.Text, Shot: shot, Seconds: speech.Round(speech.Seconds(l.Text, lineLanguage(l, c.Language), cal))})
	}
	fitted, err := s.agents.RepairScript(ctx, scene.Title, scene.Summary, c.Language, fitShots, fitLines, speakers, heardFindings(c.Record))
	if err != nil {
		return nil, fmt.Errorf("couldn't shorten the lines of the “%s” clip: %w", scene.Title, err)
	}
	rewritten := map[string]SpokenLine{}
	for _, f := range fitted {
		line := SpokenLine{ID: f.ID, CharacterID: f.CharacterID, Text: strings.TrimSpace(f.Text)}
		if len(c.Clip) > 0 && f.Shot >= 1 && f.Shot <= len(c.Clip) {
			line.ShotID = c.Clip[f.Shot-1].ID
		}
		if _, twice := rewritten[f.ID]; twice {
			return nil, fmt.Errorf("the shorter lines of “%s” can't be used: line %s came back twice", scene.Title, f.ID)
		}
		rewritten[f.ID] = line
	}
	after := []SpokenLine{}
	for _, l := range c.Before {
		if r, ok := rewritten[l.ID]; ok && inClip[l.ID] {
			kept := l // language, on-screen flag and start keep what they were
			kept.Text, kept.CharacterID, kept.ShotID = r.Text, r.CharacterID, r.ShotID
			if kept.ShotID == "" {
				kept.ShotID = l.ShotID
			}
			after = append(after, kept)
			continue
		}
		after = append(after, l)
	}
	if len(rewritten) != len(c.LineIDs) {
		return nil, fmt.Errorf("the shorter lines of “%s” can't be used: the writer returned %d lines for the clip's %d", scene.Title, len(rewritten), len(c.LineIDs))
	}
	if after, err = s.checkSpeechRepair(project, c, after); err != nil {
		return nil, fmt.Errorf("the shorter lines of “%s” can't be used: %w", scene.Title, err)
	}
	repair := &speechRepair{OutputID: outputID, SceneID: scene.ID, SceneTitle: scene.Title, ExpectedUpdatedAt: scene.UpdatedAt, Heard: heardFindings(c.Record), LineIDs: c.LineIDs, Before: c.Before, After: after,
		BeforeFit: s.sceneSpeechFor(ctx, scene, c.Before), AfterFit: s.sceneSpeechFor(ctx, scene, after), Quote: s.speechRepairQuote(ctx, scene, after)}
	if a := c.Record.Result.Audio; a != nil {
		repair.SpeechEnd, repair.ClipLength = a.SpeechEnd, a.ClipLength
	}
	return repair, nil
}

func lineLanguage(l SpokenLine, fallback string) string {
	if l.Language != "" {
		return l.Language
	}
	return fallback
}

// checkSpeechRepair checks a repaired script against the clip it repairs: every line
// of other clips is exactly as it was and in its place, the clip's lines keep their
// ids, order, speakers and shots, none is longer and together they are shorter, and the
// whole script is a valid spoken script. It returns the script as it will be saved.
func (s *Server) checkSpeechRepair(project string, c *speechRepairClip, after []SpokenLine) ([]SpokenLine, error) {
	if len(after) != len(c.Before) {
		return nil, fmt.Errorf("lines were added or removed")
	}
	inClip := map[string]bool{}
	for _, id := range c.LineIDs {
		inClip[id] = true
	}
	cal := s.speechCalibration()
	before, now := 0.0, 0.0
	for i, old := range c.Before {
		l := after[i]
		if l.ID != old.ID {
			return nil, fmt.Errorf("the lines are not in their order")
		}
		if !inClip[old.ID] {
			if !sameLines([]SpokenLine{l}, []SpokenLine{old}) {
				return nil, fmt.Errorf("line %d belongs to another clip and was changed", i+1)
			}
			continue
		}
		if l.CharacterID != old.CharacterID {
			return nil, fmt.Errorf("line %d is given to another speaker", i+1)
		}
		if old.ShotID != "" && l.ShotID != old.ShotID {
			return nil, fmt.Errorf("line %d was moved to another shot", i+1)
		}
		if l.Language != old.Language || l.Visible != old.Visible {
			return nil, fmt.Errorf("line %d changed its language or who is seen saying it", i+1)
		}
		language := lineLanguage(old, c.Language)
		was, is := speech.Seconds(old.Text, language, cal), speech.Seconds(l.Text, language, cal)
		if strings.TrimSpace(l.Text) == "" || is > was+1e-9 {
			return nil, fmt.Errorf("line %d is not shorter", i+1)
		}
		before, now = before+was, now+is
	}
	if now >= before {
		return nil, fmt.Errorf("the clip's lines are no shorter than before")
	}
	return s.validSpokenLines(project, after, c.Shots, c.Scene.Duration)
}

// withSpokenLines is a copy of the scene that says lines, for pricing it unsaved.
func withSpokenLines(scene *models.Scene, lines []SpokenLine) models.Scene {
	copy := *scene
	var snapshot map[string]json.RawMessage
	_ = json.Unmarshal(scene.SceneJSON, &snapshot) // unreadable metadata is priced without its other settings
	if snapshot == nil {
		snapshot = map[string]json.RawMessage{}
	}
	snapshot["spoken_lines"], _ = json.Marshal(lines)
	copy.SceneJSON, _ = json.Marshal(snapshot)
	return copy
}

// speechRepairQuote is the price of making the scene's clip again with lines.
func (s *Server) speechRepairQuote(ctx context.Context, scene *models.Scene, lines []SpokenLine) speechRepairQuote {
	priced := withSpokenLines(scene, lines)
	plan, err := s.sceneProductionPlan(ctx, &priced)
	if err != nil {
		return speechRepairQuote{Blocked: err.Error()}
	}
	spend := productionSpend(plan, false)
	return speechRepairQuote{PlanID: plan.ID, Calls: spend.Calls, Seconds: spend.Seconds, Storyboards: spend.Storyboards, Blocked: plan.Blocked}
}

// speechRepairApply is POST /outputs/{id}/speech-repair/apply: the repaired lines and
// the price the owner saw.
type speechRepairApply struct {
	After             []SpokenLine      `json:"after"`
	ExpectedUpdatedAt time.Time         `json:"expected_updated_at"`
	Quote             speechRepairQuote `json:"quote"`
}

// applySpeechRepair saves a repaired script (with a revision) and makes the scene's clip
// again at the price that was shown. A scene changed since the proposal, or a price
// that went up, is refused before anything is saved.
func (s *Server) applySpeechRepair(ctx context.Context, project, outputID string, body speechRepairApply) (map[string]any, int, error) {
	c, err := s.speechRepairTarget(project, outputID)
	if err != nil {
		return nil, 409, err
	}
	scene := c.Scene
	if !scene.UpdatedAt.Equal(body.ExpectedUpdatedAt) {
		return nil, 409, fmt.Errorf("“%s” changed since the shorter lines were written; ask again", scene.Title)
	}
	after, err := s.checkSpeechRepair(project, c, body.After)
	if err != nil {
		return nil, 422, fmt.Errorf("the shorter lines of “%s” can't be used: %w", scene.Title, err)
	}
	quote := s.speechRepairQuote(ctx, scene, after)
	switch {
	case quote.Blocked != "":
		return nil, 409, fmt.Errorf("“%s” can't be made again right now: %s", scene.Title, quote.Blocked)
	case quote.PlanID != body.Quote.PlanID || quote.Calls > body.Quote.Calls || quote.Seconds > body.Quote.Seconds || quote.Storyboards > body.Quote.Storyboards:
		return nil, 409, fmt.Errorf("the price of making “%s” again changed since it was shown; ask again", scene.Title)
	}
	if err = s.saveSpokenLines(scene, after); err != nil {
		return nil, 500, err
	}
	jobs, err := speechRepairRender(s, ctx, scene.ID, body.Quote)
	if err != nil {
		return nil, 502, fmt.Errorf("the shorter lines of “%s” are saved, but its video didn't start: %w", scene.Title, err)
	}
	return map[string]any{"scene_id": scene.ID, "output_id": outputID, "job_ids": jobs, "lines": after, "seconds": body.Quote.Seconds}, 0, nil
}

// speechRepairRender makes the repaired scene's video; tests replace it.
var speechRepairRender = (*Server).startSceneRender

// startSceneRender makes a scene's video at a confirmed price, as the render button
// does, and returns the started jobs.
func (s *Server) startSceneRender(ctx context.Context, sceneID string, quote speechRepairQuote) ([]string, error) {
	code, raw := s.internalCall(ctx, "POST", "/scenes/"+sceneID+"/render", map[string]any{"prepare_storyboard": true, "use_scene_cast": true, "confirmed_plan_id": quote.PlanID, "confirmed_calls": quote.Calls, "confirmed_seconds": quote.Seconds, "confirmed_storyboard_calls": quote.Storyboards})
	if code >= 300 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(raw)))
	}
	var started struct {
		JobID  string   `json:"job_id"`
		JobIDs []string `json:"job_ids"`
	}
	if err := json.Unmarshal(raw, &started); err != nil {
		return nil, fmt.Errorf("couldn't read the started video jobs: %w", err)
	}
	if len(started.JobIDs) == 0 && started.JobID != "" {
		started.JobIDs = []string{started.JobID}
	}
	return started.JobIDs, nil
}

// handleSpeechRepair is POST /outputs/{id}/speech-repair: the proposal, not saved.
func (s *Server) handleSpeechRepair(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	repair, err := s.proposeSpeechRepair(ctx, project, chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, repair)
}

// handleSpeechRepairApply is POST /outputs/{id}/speech-repair/apply.
func (s *Server) handleSpeechRepairApply(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body speechRepairApply
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&body) != nil || body.After == nil {
		http.Error(w, "send the shorter lines and the price that was shown", 400)
		return
	}
	result, code, err := s.applySpeechRepair(r.Context(), project, chi.URLParam(r, "id"), body)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// flagRepairProblem is speechRepairProblem for a flagged clip as the chat sees it.
func flagRepairProblem(f selfCheckFlag) string {
	return speechRepairProblem(&selfCheckRecord{Result: selfCheckResult{Verdict: f.Verdict, Issues: f.Issues}})
}

// resolveChatSpeechRepair points repair_clip_speech at a clip whose speech the check
// flagged: the clip the planner or the selection named, else one of the named scene,
// else the first. With none flagged, or one a shorter line cannot fix, the action
// becomes unknown with a reply saying why.
func resolveChatSpeechRepair(turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]any) string {
	if action != speechRepairAction {
		return action
	}
	output, scene := res.OutputID, res.SceneID
	if sel := turn.body.Selected; sel != nil && sel.Kind == "output" {
		output = sel.ID
	} else if sel != nil && sel.Kind == "scene" {
		scene = sel.ID
	}
	flags := []selfCheckFlag{}
	for _, f := range turn.selfChecks {
		if f.Kind == selfCheckClipAudio {
			flags = append(flags, f)
		}
	}
	var chosen *selfCheckFlag
	for i := range flags {
		switch f := &flags[i]; {
		case output != "" && f.ID == output:
			chosen = f
		case chosen == nil && output == "" && (scene == "" || f.SceneID == scene):
			chosen = f
		}
	}
	reply := ""
	switch {
	case chosen == nil && len(flags) == 0:
		reply = "The speech check hasn't flagged any clip, so there is no line to shorten."
	case chosen == nil:
		reply = "The speech check didn't flag that clip. Its flagged clips: " + flags[0].Label + "."
	default:
		if problem := flagRepairProblem(*chosen); problem != "" {
			reply = "I can't fix the " + chosen.Label + " by shortening its lines: " + problem + "."
		}
	}
	if reply != "" {
		intent["action"], intent["reply"] = "unknown", reply
		return "unknown"
	}
	intent["output_id"], intent["scene_id"], intent["clip_label"], intent["issue"] = chosen.ID, chosen.SceneID, chosen.Label, chosen.firstIssue()
	return action
}
