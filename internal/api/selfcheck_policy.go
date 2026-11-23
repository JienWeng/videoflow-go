package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// What happens after a check, per artifact cost and assistant mode:
//
//	pictures (image credits)  Full auto: a "fix"/"fail" worth retrying is made again ONCE
//	                          with the reviewer's change and checked again; a picture that
//	                          still fails (or is not worth retrying) stops the run and is
//	                          shown. Manual/Auto: checked on demand, findings shown only.
//	clips (video credits)     Full auto checks every new clip (and, for a clip made
//	                          with its own sound, listens to it: the speech check,
//	                          audio_check.go). A clip whose speech was cut off, changed
//	                          or missing gets ONE automatic repair per scene: shorter
//	                          lines, the clip made again and checked again
//	                          (speech_retry.go). Anything else flagged, or still
//	                          flagged after that retry, stops before joining and asks
//	                          the user ("Re-render with this change" / "Re-render
//	                          again" / "Keep it"). Manual/Auto: checked on demand,
//	                          findings shown only; a clip is never made again by itself.
//
// A check that cannot run reads as "pass", so it never holds work back.

// Steps the policy can choose.
const (
	selfCheckKeep   = "keep"  // use it as it is
	selfCheckShow   = "show"  // Manual/Auto: show the findings, do nothing else
	selfCheckRemake = "retry" // Full auto picture: make it again once with the change
	selfCheckStop   = "stop"  // Full auto picture: stop the run and show the findings
	selfCheckAsk    = "ask"   // Full auto clip: ask whether to re-render or keep it
)

// pictureSelfCheckStep is what follows a picture's check.
func pictureSelfCheckStep(fullAuto bool, r selfCheckRecord) string {
	switch {
	case r.passes():
		return selfCheckKeep
	case !fullAuto:
		return selfCheckShow
	case r.Attempt < 2 && r.Result.Retry.WorthIt && strings.TrimSpace(r.Result.Retry.Change) != "":
		return selfCheckRemake
	default:
		return selfCheckStop
	}
}

// clipSelfCheckStep is what follows a clip's check. A video is never made again by
// itself: it costs video credits, so the user always decides.
func clipSelfCheckStep(fullAuto bool, r selfCheckRecord, acceptedInLog bool) string {
	switch {
	case r.passes() || acceptedInLog:
		return selfCheckKeep
	case !fullAuto:
		return selfCheckShow
	default:
		return selfCheckAsk
	}
}

// selfCheckDecisionKey is the decision-log key the "Re-render or keep" question settles.
func selfCheckDecisionKey(outputID string) string { return "selfcheck:" + outputID }

// clipKeptInLog says whether the user answered "Keep it" for this clip in the chat.
func clipKeptInLog(decisions []db.ProjectDecision, outputID string) bool {
	key := selfCheckDecisionKey(outputID)
	for _, d := range decisions {
		if d.Key == key && strings.HasPrefix(strings.ToLower(strings.TrimSpace(d.Value)), "keep") {
			return true
		}
	}
	return false
}

// selfCheckFlag is one artifact whose check found something the user should see.
type selfCheckFlag struct {
	Kind    string           `json:"kind"`
	ID      string           `json:"id"`
	Label   string           `json:"label"`
	SceneID string           `json:"scene_id,omitempty"`
	Verdict string           `json:"verdict"`
	Score   int              `json:"score"`
	Issues  []selfCheckIssue `json:"issues"`
	// Change is the reviewer's suggested prompt change.
	Change     string `json:"change,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
	// Problem is set when an automatic retry could not be made.
	Problem string `json:"problem,omitempty"`
	// Retried is set on a speech flag whose clip already had Full auto's one automatic
	// retry (shorter lines, made again): it is asked about, never retried again.
	Retried bool `json:"retried,omitempty"`
}

func newSelfCheckFlag(t *selfCheckTarget, r selfCheckRecord) selfCheckFlag {
	return selfCheckFlag{Kind: t.Kind, ID: t.ID, Label: t.Label, SceneID: t.SceneID, Verdict: r.Result.Verdict, Score: r.Result.Score, Issues: r.Result.Issues, Change: r.Result.Retry.Change, Suggestion: r.Result.Suggestion, Attempt: r.Attempt}
}

// firstIssue is the most important finding, in words.
func (f selfCheckFlag) firstIssue() string {
	if len(f.Issues) > 0 {
		return f.Issues[0].Detail
	}
	if f.Suggestion != "" {
		return f.Suggestion
	}
	return "it doesn't match what was asked for"
}

// clipReviewDecision is the question Full auto asks about a flagged clip, in the chat's
// decision schema: re-render it with the reviewer's change (video credits) or keep it.
func clipReviewDecision(f selfCheckFlag) chatDecision {
	change := f.Change
	if change == "" {
		change = "fix this: " + f.firstIssue()
	}
	checker, ask, label, tried := "check", "Re-render it with a fix?", "Re-render with this change", false
	if f.Kind == selfCheckClipAudio {
		checker = "speech check"
		if hasIssue(f.Issues, "speech_cut_off") {
			ask, label = "Re-render it with a shorter line?", "Re-render with a shorter line"
		}
		if f.Retried {
			tried, ask, label = true, "Re-render it again?", "Re-render again"
		}
	}
	question := fmt.Sprintf("The %s flagged the %s: %s %s That uses video credits.", checker, f.Label, strings.TrimRight(f.firstIssue(), ".")+".", ask)
	if tried {
		question = fmt.Sprintf("I already shortened its lines and re-rendered it once, and the %s still flagged the %s: %s %s That uses video credits.", checker, f.Label, strings.TrimRight(f.firstIssue(), ".")+".", ask)
	}
	return chatDecision{
		Mode:        decisionAsk,
		Assumptions: []string{},
		Question:    clip(question, maxQuestionChars),
		Options: []chatDecisionOption{
			{Label: label, Value: clip(fmt.Sprintf("Re-render the %s with this change: %s", strings.TrimSuffix(f.Label, "’s speech"), change), maxOptionValue)},
			{Label: "Keep it", Value: clip("Keep the "+strings.TrimSuffix(f.Label, "’s speech")+" as it is.", maxOptionValue)},
		},
		Reason: askExpensive,
		Key:    selfCheckDecisionKey(f.ID),
	}
}

// reRenderChatAction says whether an action makes a scene's video again.
func reRenderChatAction(action string) bool {
	return action == "render_scene" || action == "render_scenes" || action == "retry_render"
}

// clipReviewAsk says whether a question is the flagged-clip question Full auto allows:
// it is about the cost of re-rendering a clip the check flagged.
func clipReviewAsk(d chatDecision, action string, clipFlagged bool) bool {
	return clipFlagged && d.Reason == askExpensive && (strings.HasPrefix(d.Key, "selfcheck:") || reRenderChatAction(action))
}

// --- Clips the project currently uses ---

// currentSceneClipJobs is, by scene, the finished scene-level jobs of the newest complete
// render: every unit of one render plan, or a single job made without a plan.
func currentSceneClipJobs(jobs []models.RenderJob) map[string][]models.RenderJob {
	type group struct {
		scene  string
		count  int
		units  map[int]models.RenderJob
		single bool
	}
	groups := map[string]*group{}
	for _, job := range jobs {
		if job.Status != models.StatusSucceeded || job.SceneID == nil || job.ShotID != nil {
			continue
		}
		r := renderRequestRef(job.RequestJSON)
		if r.Unit.Key == "" {
			groups["job:"+job.ID] = &group{scene: *job.SceneID, count: 1, units: map[int]models.RenderJob{0: job}, single: true}
			continue
		}
		plan := r.Plan
		if plan == "" {
			plan = strings.SplitN(r.Unit.Key, ":", 2)[0]
		}
		if plan == "" || r.Unit.Count < 1 || r.Unit.Index < 0 || r.Unit.Index >= r.Unit.Count {
			continue
		}
		key := *job.SceneID + ":" + plan
		g := groups[key]
		if g == nil {
			g = &group{scene: *job.SceneID, count: r.Unit.Count, units: map[int]models.RenderJob{}}
			groups[key] = g
		}
		if g.count == r.Unit.Count {
			if old, ok := g.units[r.Unit.Index]; !ok || job.UpdatedAt.After(old.UpdatedAt) {
				g.units[r.Unit.Index] = job
			}
		}
	}
	newest := map[string]*group{}
	newestAt := func(g *group) (latest models.RenderJob) {
		for _, j := range g.units {
			if j.UpdatedAt.After(latest.UpdatedAt) {
				latest = j
			}
		}
		return latest
	}
	for _, g := range groups {
		if len(g.units) != g.count {
			continue
		}
		if best := newest[g.scene]; best == nil || newestAt(g).UpdatedAt.After(newestAt(best).UpdatedAt) {
			newest[g.scene] = g
		}
	}
	out := map[string][]models.RenderJob{}
	for scene, g := range newest {
		for i := 0; i < g.count; i++ {
			out[scene] = append(out[scene], g.units[i])
		}
	}
	return out
}

// stageClip is one clip the project currently uses, with its last checks.
type stageClip struct {
	SceneID, SceneTitle, OutputID string
	Check                         *selfCheckRecord
	// Audio is the clip's last speech check.
	Audio *selfCheckRecord
	// Kept and AudioKept are set when the user kept the clip despite the findings.
	Kept, AudioKept bool
}

// stageClips lists the clips of each scene's newest finished video, in scene order.
func (s *Server) stageClips(scenes []stageScene, jobs []models.RenderJob, decisions []db.ProjectDecision) []stageClip {
	current := currentSceneClipJobs(jobs)
	clips := []stageClip{}
	for _, sc := range scenes {
		for _, job := range current[sc.ID] {
			outs, _ := s.database.ListOutputsForJob(job.ID) // an unreadable clip has no check to show
			for _, out := range outs {
				c := stageClip{SceneID: sc.ID, SceneTitle: sc.Title, OutputID: out.ID, Check: storedSelfCheck(out.QAJSON), Audio: storedAudioCheck(out.QAJSON)}
				kept := clipKeptInLog(decisions, out.ID)
				c.Kept = kept || c.Check != nil && c.Check.AcceptedAt != nil
				c.AudioKept = kept || c.Audio != nil && c.Audio.AcceptedAt != nil
				clips = append(clips, c)
			}
		}
	}
	return clips
}

// flagged says whether the clip's checks found problems the user has not settled.
func (c stageClip) flagged() bool { return c.lookFlagged() || c.speechFlagged() }

// lookFlagged is the frames check's part, speechFlagged the speech check's.
func (c stageClip) lookFlagged() bool   { return c.Check != nil && !c.Check.passes() && !c.Kept }
func (c stageClip) speechFlagged() bool { return c.Audio != nil && !c.Audio.passes() && !c.AudioKept }

// stageFinding is one clip's check as the review stage shows it.
type stageFinding struct {
	// Kind is clip (the frames) or clip_audio (the speech check).
	Kind     string           `json:"kind"`
	SceneID  string           `json:"scene_id"`
	OutputID string           `json:"output_id"`
	Label    string           `json:"label"`
	Verdict  string           `json:"verdict"`
	Score    int              `json:"score"`
	Issues   []selfCheckIssue `json:"issues,omitempty"`
	Change   string           `json:"change,omitempty"`
	Kept     bool             `json:"kept,omitempty"`
	Note     string           `json:"note,omitempty"`
}

// --- Full auto in the autopilot run ---

// selfCheckChangeKey carries a reviewer's change into a picture's generation prompt.
type selfCheckChangeKey struct{}

// selfCheckCorrection is the prompt line a retried picture gets.
func selfCheckCorrection(change string) string {
	return "\nCorrection from the picture check: " + change
}

// autopilotPictureTargets are the pictures the run's videos are made from: the cast's
// generated character sheets and each scene's current storyboard. Uploaded pictures
// are the user's own and are never checked automatically.
func (a *autopilotRunner) autopilotPictureTargets(ctx context.Context) []*selfCheckTarget {
	targets, seen := []*selfCheckTarget{}, map[string]bool{}
	for _, scene := range a.scenes {
		for _, id := range models.IDList(scene.CharacterIDsJSON) {
			c, err := a.s.database.GetCharacter(id)
			if err != nil || c.ProjectID == nil || *c.ProjectID != a.project {
				continue
			}
			for _, ref := range models.IDList(c.ReferenceAssetIDsJSON) {
				asset, err := a.s.database.GetAsset(ref)
				if err != nil || seen[ref] || asset.Type != "character_reference" || !models.ReferenceApproved(asset.MetadataJSON) {
					continue
				}
				if t, err := a.s.characterSheetTarget(a.project, ref); err == nil {
					seen[ref] = true
					targets = append(targets, t)
				}
			}
		}
	}
	for i := range a.scenes {
		state, err := a.s.sceneReferenceState(ctx, &a.scenes[i])
		if err != nil {
			continue
		}
		if board, _ := state["active_storyboard_id"].(string); board != "" && !seen[board] {
			if t, err := a.s.scenePictureTarget(a.project, board); err == nil {
				seen[board] = true
				targets = append(targets, t)
			}
		}
	}
	return targets
}

// checkPictures is Full auto's picture check, before any video is paid for: each
// picture is checked, one worth retrying is made again once with the reviewer's change
// and checked again, and anything still flagged stops the run with its findings.
func (a *autopilotRunner) checkPictures(ctx context.Context) (any, error) {
	if !a.body.FullAuto || a.body.JoinOnly {
		return nil, nil
	}
	flagged := []selfCheckFlag{}
	for _, t := range a.autopilotPictureTargets(ctx) {
		if err := a.sameProject(); err != nil {
			return nil, err
		}
		a.stage("Checking " + t.Label)
		record, _, err := a.s.runSelfCheck(ctx, t, selfCheckOptions{})
		if err != nil {
			continue // a check that cannot run never holds the video back
		}
		switch pictureSelfCheckStep(true, record) {
		case selfCheckRemake:
			a.stage("Making " + t.Label + " again with the check's fix")
			retried, err := selfCheckRemakePicture(a.s, ctx, t, record.Result.Retry.Change)
			if err != nil {
				f := newSelfCheckFlag(t, record)
				f.Problem = "couldn't make it again: " + err.Error()
				flagged = append(flagged, f)
				continue
			}
			a.stage("Checking the new " + retried.Label)
			again, _, err := a.s.runSelfCheck(ctx, retried, selfCheckOptions{Attempt: 2})
			if err == nil && pictureSelfCheckStep(true, again) != selfCheckKeep {
				flagged = append(flagged, newSelfCheckFlag(retried, again))
			}
		case selfCheckStop:
			flagged = append(flagged, newSelfCheckFlag(t, record))
		}
	}
	if len(flagged) == 0 {
		return nil, nil
	}
	f := flagged[0]
	message := fmt.Sprintf("The picture check flagged the %s: %s", f.Label, f.firstIssue())
	if f.Attempt >= 2 {
		message += " It was already made again once."
	}
	if len(flagged) > 1 {
		message += fmt.Sprintf(" (%d pictures need a look.)", len(flagged))
	}
	return map[string]any{"stage": "Needs your review", "self_check": flagged}, fmt.Errorf("%s No video was made. Keep the picture or make it again, then carry on", strings.TrimRight(message, ".")+".")
}

// selfCheckRemakePicture makes a picture again; tests replace it.
var selfCheckRemakePicture = (*Server).retryPicture

// retryPicture makes a flagged picture again with the reviewer's change and returns
// the new picture to check.
func (s *Server) retryPicture(ctx context.Context, t *selfCheckTarget, change string) (*selfCheckTarget, error) {
	ctx = context.WithValue(withDirectorTurnGate(ctx), selfCheckChangeKey{}, change)
	switch t.Kind {
	case selfCheckCharacterSheet:
		c, err := s.database.GetCharacter(t.CharacterID)
		if err != nil || c.ProjectID == nil || *c.ProjectID != t.ProjectID {
			return nil, fmt.Errorf("the character is gone")
		}
		made, err := s.generateCharacterSheet(guidedStoryContext(ctx, t.ProjectID, []string{c.ID}), c, false, "")
		if err != nil {
			return nil, err
		}
		for _, id := range models.IDList(made.ReferenceAssetIDsJSON) {
			if id == t.ID {
				continue
			}
			if asset, err := s.database.GetAsset(id); err == nil && asset.Type == "character_reference" {
				return s.characterSheetTarget(t.ProjectID, id)
			}
		}
		return nil, fmt.Errorf("no new picture was made")
	case selfCheckScenePicture:
		if t.SceneID == "" {
			return nil, fmt.Errorf("the picture belongs to no scene")
		}
		unlock, err := s.lockSceneRender(t.SceneID)
		if err != nil {
			return nil, err
		}
		defer unlock()
		scene, err := s.database.GetScene(t.SceneID)
		if err != nil || scene.ProjectID == nil || *scene.ProjectID != t.ProjectID {
			return nil, fmt.Errorf("the scene is gone")
		}
		assets, err := s.storyboardCandidates(guidedStoryContext(ctx, t.ProjectID, nil), scene, selfCheckCorrection(change))
		if err != nil {
			return nil, err
		}
		if _, err = s.attachStoryboard(scene, assets); err != nil {
			return nil, err
		}
		return s.scenePictureTarget(t.ProjectID, assets[0].ID)
	}
	return nil, fmt.Errorf("only pictures are made again automatically")
}

// checkClips is Full auto's clip check once the videos are made: every clip the
// scenes now use is checked (an unchanged one is not checked twice). It never makes a
// clip again; it returns the flagged clips the user has not kept.
func (a *autopilotRunner) checkClips(ctx context.Context) []selfCheckFlag {
	if !a.body.FullAuto {
		return nil
	}
	jobs, _ := a.s.database.ListRenderJobs(a.project)     // no jobs means no clips to check
	decisions, _ := a.s.database.ListDecisions(a.project) // an unreadable log keeps nothing
	current := currentSceneClipJobs(jobs)
	flagged := []selfCheckFlag{}
	for _, scene := range a.scenes {
		for _, job := range current[scene.ID] {
			outs, _ := a.s.database.ListOutputsForJob(job.ID) // an unreadable clip is not checked
			for _, out := range outs {
				t, err := a.s.clipTarget(a.project, out.ID)
				if err != nil {
					continue
				}
				a.stage("Checking the " + t.Label)
				record, _, err := a.s.runSelfCheck(ctx, t, selfCheckOptions{})
				if err == nil && clipSelfCheckStep(true, record, clipKeptInLog(decisions, out.ID)) == selfCheckAsk {
					flagged = append(flagged, newSelfCheckFlag(t, record))
				}
				// A clip with its own sound is listened to: were the lines said, whole,
				// in the right language? The transcript is kept for the captions.
				if !nativeSound(&job) {
					continue
				}
				heard, err := a.s.clipAudioTarget(a.project, out.ID)
				if err != nil {
					continue
				}
				a.stage("Listening to the " + t.Label)
				record, _, err = a.s.runSelfCheck(ctx, heard, selfCheckOptions{})
				if err == nil && clipSelfCheckStep(true, record, clipKeptInLog(decisions, out.ID)) == selfCheckAsk {
					flagged = append(flagged, newSelfCheckFlag(heard, record))
				}
			}
		}
	}
	return flagged
}

// clipReviewResult is how a Full auto run ends when the clip check flagged something:
// nothing more is made or joined, and the first flagged clip is asked about.
func (a *autopilotRunner) clipReviewResult(flagged []selfCheckFlag) map[string]any {
	sort.SliceStable(flagged, func(i, j int) bool { return flagged[i].Score < flagged[j].Score })
	return map[string]any{"stage": "Needs your review", "needs_review": true, "videos_only": a.body.VideosOnly, "script_id": a.body.ScriptID, "self_check": flagged, "decision": clipReviewDecision(flagged[0])}
}

// --- The chat turn ---

// flaggedClips are the clips whose checks found problems the user has not settled.
func flaggedClips(clips []stageClip) []selfCheckFlag {
	out := []selfCheckFlag{}
	flag := func(c stageClip, kind, label string, r *selfCheckRecord) selfCheckFlag {
		return selfCheckFlag{Kind: kind, ID: c.OutputID, Label: label, SceneID: c.SceneID, Verdict: r.Result.Verdict, Score: r.Result.Score, Issues: r.Result.Issues, Change: r.Result.Retry.Change, Suggestion: r.Result.Suggestion, Attempt: r.Attempt}
	}
	for _, c := range clips {
		if c.lookFlagged() {
			out = append(out, flag(c, selfCheckClip, "“"+c.SceneTitle+"” clip", c.Check))
		}
		if c.speechFlagged() {
			out = append(out, flag(c, selfCheckClipAudio, "“"+c.SceneTitle+"” clip’s speech", c.Audio))
		}
	}
	return out
}

// flaggedPictures are the project's current pictures (characters' bound sheets and
// scenes' linked storyboards) whose check found problems the user has not kept.
func (s *Server) flaggedPictures(projectID string, chars []models.Character, scenes []models.Scene) []selfCheckFlag {
	current := map[string]string{} // asset id → label
	for _, c := range chars {
		for _, id := range models.IDList(c.ReferenceAssetIDsJSON) {
			current[id] = c.Name + "’s picture"
		}
	}
	sceneOf := map[string]string{}
	for _, sc := range scenes {
		for _, id := range models.IDList(sc.AssetIDsJSON) {
			current[id], sceneOf[id] = "“"+sc.Title+"” picture", sc.ID
		}
	}
	assets, _ := s.database.ListAssets(projectID) // no readable assets means nothing flagged
	out := []selfCheckFlag{}
	for _, a := range assets {
		label, ok := current[a.ID]
		record := storedSelfCheck(a.MetadataJSON)
		if !ok || record == nil || record.passes() {
			continue
		}
		kind := selfCheckScenePicture
		if a.Type == "character_reference" {
			kind = selfCheckCharacterSheet
		}
		out = append(out, selfCheckFlag{Kind: kind, ID: a.ID, Label: label, SceneID: sceneOf[a.ID], Verdict: record.Result.Verdict, Score: record.Result.Score, Issues: record.Result.Issues, Change: record.Result.Retry.Change, Suggestion: record.Result.Suggestion, Attempt: record.Attempt})
		if len(out) == 8 {
			break
		}
	}
	return out
}

// clipFlagged says whether a clip the self-check flagged is waiting on the user's
// answer. Around speech only two questions remain in Full auto: after its one
// automatic retry, and for what a shorter line can't fix (wrong language, no speech).
// A speech flag that still has its retry to come is not asked about: Full auto
// repairs it (repair_clip_speech) instead.
func (t *chatTurn) clipFlagged() bool {
	for _, f := range t.selfChecks {
		if f.Kind == selfCheckClip || f.Kind == selfCheckClipAudio && (f.Retried || flagRepairProblem(f) != "") {
			return true
		}
	}
	return false
}

// selfCheckRule tells the planner how to speak to the self-check's findings.
const selfCheckRule = "Findings of the app's own checks of pictures and clips: kind clip is the frames, looked at by the local AI session (no credits); kind clip_audio is the speech check, which listened to the clip's sound and compared it with the script (said or not, the words, the language, cut off at the end). Speak to them plainly when relevant. For a clip_audio finding marked repairable (lines cut off, changed or missing), offer repair_clip_speech with its id as output_id: it rewrites only that clip's lines shorter and simpler (free) and makes the scene's clip again (video credits). For any other flagged clip, ask once with decision mode ask, reason expensive, key decision_key, options \"Re-render with this change\" (value: re-render that scene with the change) and \"Keep it\"; never re-render it yourself. Lip-sync is never checked, and without a clip_audio finding nothing was heard, so never claim anything about the sound."

// chatSelfCheck is the self-check part of a chat turn's context.
func chatSelfCheck(flags []selfCheckFlag) map[string]any {
	items := []map[string]any{}
	for _, f := range flags {
		item := map[string]any{"kind": f.Kind, "id": f.ID, "label": f.Label, "verdict": f.Verdict, "score": f.Score, "issue": f.firstIssue()}
		if f.SceneID != "" {
			item["scene_id"] = f.SceneID
		}
		if f.Change != "" {
			item["change"] = f.Change
		}
		if f.Kind == selfCheckClip || f.Kind == selfCheckClipAudio {
			item["decision_key"] = selfCheckDecisionKey(f.ID)
		}
		if f.Kind == selfCheckClipAudio {
			item["repairable"] = flagRepairProblem(f) == "" && !f.Retried
			if f.Retried {
				item["auto_retried"] = true
			}
		}
		items = append(items, item)
	}
	return map[string]any{"rule": selfCheckRule, "flagged": items}
}
