package api

import (
	"context"
	"fmt"
	"strings"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// Full auto's one automatic speech retry. After the clips are made and listened to, a
// clip the speech check flagged for a line cut off, said with other words or missing
// gets ONE repair by itself: its lines are rewritten shorter (speech_repair.go), the
// scene's clip is made again and listened to again. If it is still flagged, the run
// stops and asks, saying it already tried once.
//
// The limit is kept in the decision log, not in memory: speech_retry:<output id> is
// written (by the assistant) right before the new clip is paid for, and a scene with
// any such entry for one of its clips never gets another automatic retry, after a
// reload, in a second run, or in the chat. A scene's clips share one render plan, so
// the repair makes all of them again; "once per clip" is therefore once per scene.
//
// Never retried by itself: wrong language and no speech (a shorter line can't fix
// them; asked instead), a check that couldn't listen (it reads as a pass), a clip the
// frames check flagged as a real person, a scene whose video was refused by the
// service's safety filters, and anything outside Full auto.

// speechRetryKey is the decision-log key of a clip's automatic speech retry.
func speechRetryKey(outputID string) string { return "speech_retry:" + outputID }

// speechRetriedScenes are the scenes one of whose clips already had its automatic
// speech retry, from the decision log.
func (s *Server) speechRetriedScenes(decisions []db.ProjectDecision) map[string]bool {
	scenes := map[string]bool{}
	for _, d := range decisions {
		id, ok := strings.CutPrefix(d.Key, "speech_retry:")
		if !ok {
			continue
		}
		out, err := s.database.GetRenderOutput(id)
		if err != nil {
			continue
		}
		if job, err := s.database.GetRenderJob(out.RenderJobID); err == nil && job.SceneID != nil {
			scenes[*job.SceneID] = true
		}
	}
	return scenes
}

// recordSpeechRetry writes the clip's automatic retry into the decision log.
func (s *Server) recordSpeechRetry(project, outputID, title string) error {
	_, err := s.database.SetDecision(project, speechRetryKey(outputID), "Full auto shortened the lines of the “"+title+"” clip and re-rendered it once", db.DecisionByAssistant, false)
	return err
}

// safetyWords mark a video the service refused for what it shows or says.
var safetyWords = []string{"safety", "real person", "prohibited", "blocked", "usage polic"}

// sceneSafetyBlocked says whether the video service refused one of the scene's videos
// for safety: making it again by itself would only be refused again.
func (s *Server) sceneSafetyBlocked(project, sceneID string) bool {
	jobs, _ := s.database.ListRenderJobs(project) // unreadable jobs block nothing
	for _, j := range jobs {
		if j.SceneID == nil || *j.SceneID != sceneID || j.Status != models.StatusFailed || j.Error == nil {
			continue
		}
		text := strings.ToLower(*j.Error)
		for _, w := range safetyWords {
			if strings.Contains(text, w) {
				return true
			}
		}
	}
	return false
}

// speechRetryProblem says why a clip may not get its automatic speech retry, or "".
func (s *Server) speechRetryProblem(project, sceneID, outputID string) string {
	out, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		return "video not found"
	}
	if problem := speechRepairProblem(storedAudioCheck(out.QAJSON)); problem != "" {
		return problem
	}
	if look := storedSelfCheck(out.QAJSON); look != nil && hasIssue(look.Result.Issues, "real_person_risk") {
		return "the frames check says this clip may show a real person, so it is not made again by itself"
	}
	if s.sceneSafetyBlocked(project, sceneID) {
		return "the video service refused this scene for safety before, so it is not made again by itself"
	}
	decisions, _ := s.database.ListDecisions(project) // an unreadable log is read as no retry yet
	if s.speechRetriedScenes(decisions)[sceneID] {
		return "Full auto already shortened this clip's lines and re-rendered it once; keep it or re-render it again yourself"
	}
	return ""
}

// markSpeechRetried notes on each speech flag whose scene already had its automatic
// retry that it did, so the question says so.
func markSpeechRetried(flags []selfCheckFlag, retried map[string]bool) []selfCheckFlag {
	for i := range flags {
		if flags[i].Kind == selfCheckClipAudio && retried[flags[i].SceneID] {
			flags[i].Retried = true
		}
	}
	return flags
}

// shortenClipLines rewrites a flagged clip's lines shorter and saves them.
func (s *Server) shortenClipLines(ctx context.Context, project, outputID string) error {
	repair, err := s.proposeSpeechRepair(ctx, project, outputID)
	if err != nil {
		return err
	}
	scene, after, _, err := s.saveSpeechRepair(project, outputID, repair.After, repair.ExpectedUpdatedAt)
	if err != nil {
		return err
	}
	return s.saveSpokenLines(scene, after)
}

// retryClipSpeech is Full auto's bounded retry, after checkClips: each scene with a
// clip whose speech can be repaired, and that has not had its retry, gets its lines
// shortened and its clip made again ONCE, then every clip is checked again. It
// returns what is still flagged, the speech flags marked as already retried.
func (a *autopilotRunner) retryClipSpeech(ctx context.Context, flagged []selfCheckFlag) ([]selfCheckFlag, any, error) {
	if !a.body.FullAuto || a.body.JoinOnly {
		return flagged, nil, nil
	}
	decisions, _ := a.s.database.ListDecisions(a.project) // an unreadable log is read as no retry yet
	retried := a.s.speechRetriedScenes(decisions)
	todo := map[string][]selfCheckFlag{}
	for _, f := range flagged {
		if f.Kind == selfCheckClipAudio && !retried[f.SceneID] && a.s.speechRetryProblem(a.project, f.SceneID, f.ID) == "" {
			todo[f.SceneID] = append(todo[f.SceneID], f)
		}
	}
	if len(todo) == 0 {
		return markSpeechRetried(flagged, retried), nil, nil
	}
	waiting, problems := map[string][]string{}, map[string]string{}
	for i := range a.scenes {
		scene := &a.scenes[i]
		flags := todo[scene.ID]
		if len(flags) == 0 {
			continue
		}
		if err := a.sameProject(); err != nil {
			return nil, nil, err
		}
		a.stage("Shortening the lines of the “" + scene.Title + "” clip")
		failed := ""
		for _, f := range flags {
			if err := a.s.shortenClipLines(ctx, a.project, f.ID); err != nil {
				failed = "couldn't shorten its lines: " + err.Error()
				break
			}
		}
		if failed == "" {
			failed = a.rerenderScene(ctx, scene, flags[0].ID, waiting)
		}
		if failed != "" {
			for _, f := range flags {
				problems[f.ID] = failed
			}
		}
	}
	decisions, _ = a.s.database.ListDecisions(a.project) // what was recorded is what counts
	retried = a.s.speechRetriedScenes(decisions)
	if len(waiting) > 0 {
		if stuck, err := a.waitForSceneVideos(ctx, waiting); err != nil {
			return nil, stuck, err
		}
	}
	again := a.checkClips(ctx)
	for _, f := range flagged {
		if problems[f.ID] == "" {
			continue
		}
		f.Problem = problems[f.ID]
		found := false
		for i := range again {
			if again[i].ID == f.ID && again[i].Kind == f.Kind {
				again[i].Problem, found = f.Problem, true
			}
		}
		if !found {
			again = append(again, f)
		}
	}
	return markSpeechRetried(again, retried), nil, nil
}

// rerenderScene makes a scene's clip again with its saved shorter lines, recording the
// retry first, and adds the started jobs to waiting. It returns why it couldn't.
func (a *autopilotRunner) rerenderScene(ctx context.Context, scene *models.Scene, outputID string, waiting map[string][]string) string {
	fresh, err := a.s.database.GetScene(scene.ID)
	if err != nil {
		return "the scene is gone"
	}
	plan, err := a.s.sceneProductionPlan(ctx, fresh)
	if err == nil && plan.Blocked != "" {
		err = fmt.Errorf("%s", plan.Blocked)
	}
	if err != nil {
		return "couldn't make it again: " + err.Error()
	}
	if err = a.s.recordSpeechRetry(a.project, outputID, scene.Title); err != nil {
		return "couldn't note the retry, so it wasn't made again: " + err.Error()
	}
	a.stage("Making the “" + scene.Title + "” clip again with the shorter lines")
	spend := productionSpend(plan, false)
	jobs, err := speechRepairRender(a.s, ctx, scene.ID, speechRepairQuote{PlanID: plan.ID, Calls: spend.Calls, Seconds: spend.Seconds, Storyboards: spend.Storyboards})
	if err != nil {
		return "couldn't make it again: " + err.Error()
	}
	waiting[scene.ID] = jobs
	return ""
}
