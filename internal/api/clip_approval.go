package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// A clip (one finished render output) is approved before the final video is exported.
// Approval is a note on the clip (qa_json.approval), so it never changes the clip and a
// new take starts unapproved. Only a finished clip whose checks passed (or whose
// findings the user kept) can be approved; the export refuses an edit that uses an
// unapproved clip (compositionApprovalProblem).

const (
	approvalByUser     = "user"
	approvalByFullAuto = "full_auto"
	approvalByFinish   = "finish_video"
)

// approveClipsAction is the chat action that approves every passed clip at once.
const approveClipsAction = "approve_clips"

type clipApproval struct {
	Status string    `json:"status"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
}

// clipApproved says whether a clip's notes hold an approval.
func clipApproved(qa json.RawMessage) bool {
	var notes struct {
		Approval *clipApproval `json:"approval"`
	}
	return json.Unmarshal(qa, &notes) == nil && notes.Approval != nil && notes.Approval.Status == "approved"
}

// clipApprovalEntry is one clip in an approval report.
type clipApprovalEntry struct {
	OutputID string `json:"output_id,omitempty"`
	SceneID  string `json:"scene_id,omitempty"`
	Label    string `json:"label"`
	Reason   string `json:"reason,omitempty"`
}

// clipApprovalReport is what Approve all did.
type clipApprovalReport struct {
	Approved        []clipApprovalEntry `json:"approved"`
	AlreadyApproved []clipApprovalEntry `json:"already_approved"`
	Skipped         []clipApprovalEntry `json:"skipped"`
	// Unchecked are approved clips no check has looked at yet (nothing flagged them).
	Unchecked []clipApprovalEntry `json:"unchecked,omitempty"`
}

// approvedCount is every clip that is now approved.
func (r clipApprovalReport) approvedCount() int { return len(r.Approved) + len(r.AlreadyApproved) }

// message says what happened, in words.
func (r clipApprovalReport) message() string {
	parts := []string{}
	switch n := len(r.Approved); {
	case n == 1:
		parts = append(parts, "Approved 1 clip.")
	case n > 1:
		parts = append(parts, fmt.Sprintf("Approved %d clips.", n))
	}
	if n := len(r.AlreadyApproved); n > 0 {
		parts = append(parts, fmt.Sprintf("%d already approved.", n))
	}
	if len(r.Skipped) > 0 {
		reasons := []string{}
		for _, s := range r.Skipped {
			reasons = append(reasons, s.Label+" ("+s.Reason+")")
		}
		parts = append(parts, fmt.Sprintf("Not approved: %s.", strings.Join(reasons, "; ")))
	}
	if len(parts) == 0 {
		return "There are no finished clips to approve yet."
	}
	return strings.Join(parts, " ")
}

// clipLabel names a clip the way the Videos page does.
func clipLabel(title string, job *models.RenderJob) string {
	if title == "" {
		title = "Scene video"
	}
	if u := renderRequestUnit(job.RequestJSON); u.Count > 1 {
		return fmt.Sprintf("“%s” part %d of %d", title, u.Index+1, u.Count)
	}
	return "“" + title + "”"
}

// clipApprovalBlock says why a clip cannot be approved, or "" when it can: it must be
// finished and no check may have flagged it unless the user kept it.
func clipApprovalBlock(job *models.RenderJob, out *models.RenderOutput, decisions []db.ProjectDecision) string {
	if job.Status != models.StatusSucceeded {
		return "not finished"
	}
	c := stageClip{OutputID: out.ID, Check: storedSelfCheck(out.QAJSON), Audio: storedAudioCheck(out.QAJSON)}
	kept := clipKeptInLog(decisions, out.ID)
	c.Kept = kept || c.Check != nil && c.Check.AcceptedAt != nil
	c.AudioKept = kept || c.Audio != nil && c.Audio.AcceptedAt != nil
	if c.lookFlagged() {
		f := selfCheckFlag{Issues: c.Check.Result.Issues, Suggestion: c.Check.Result.Suggestion}
		return "the picture check flagged it: " + strings.TrimRight(f.firstIssue(), ".")
	}
	if c.speechFlagged() {
		f := selfCheckFlag{Issues: c.Audio.Result.Issues, Suggestion: c.Audio.Result.Suggestion}
		return "the speech check flagged it: " + strings.TrimRight(f.firstIssue(), ".")
	}
	return ""
}

// approveClipsRequest chooses the clips: named clips, or the current clips of the
// chosen scenes, of a story, or (nothing named) of the whole project.
type approveClipsRequest struct {
	ScriptID  string   `json:"script_id"`
	SceneIDs  []string `json:"scene_ids"`
	OutputIDs []string `json:"output_ids"`
}

// approveClips approves every chosen clip that may be approved and reports the rest.
func (s *Server) approveClips(project string, body approveClipsRequest, by string) (clipApprovalReport, error) {
	report := clipApprovalReport{Approved: []clipApprovalEntry{}, AlreadyApproved: []clipApprovalEntry{}, Skipped: []clipApprovalEntry{}}
	jobs, err := s.database.ListRenderJobs(project)
	if err != nil {
		return report, err
	}
	decisions, _ := s.database.ListDecisions(project) // an unreadable log keeps nothing
	all, err := s.database.ListScenes(project)
	if err != nil {
		return report, err
	}
	titles := map[string]string{}
	for _, sc := range all {
		titles[sc.ID] = sc.Title
	}
	type candidate struct {
		job *models.RenderJob
		out models.RenderOutput
	}
	candidates := []candidate{}
	if len(body.OutputIDs) > 0 {
		for _, id := range body.OutputIDs {
			out, e := s.database.GetRenderOutput(id)
			if e != nil {
				report.Skipped = append(report.Skipped, clipApprovalEntry{OutputID: id, Label: "a clip", Reason: "not found"})
				continue
			}
			job, e := s.database.GetRenderJob(out.RenderJobID)
			if e != nil || job.ProjectID == nil || *job.ProjectID != project {
				report.Skipped = append(report.Skipped, clipApprovalEntry{OutputID: id, Label: "a clip", Reason: "not in this project"})
				continue
			}
			candidates = append(candidates, candidate{job, *out})
		}
	} else {
		wanted := map[string]bool{}
		for _, id := range body.SceneIDs {
			wanted[id] = true
		}
		scenes := []models.Scene{}
		for _, sc := range all {
			inStory := body.ScriptID == "" || sc.ScriptID != nil && *sc.ScriptID == body.ScriptID
			if inStory && (len(wanted) == 0 || wanted[sc.ID]) {
				scenes = append(scenes, sc)
			}
		}
		sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].SceneOrder < scenes[j].SceneOrder })
		current := currentSceneClipJobs(jobs)
		for _, sc := range scenes {
			if len(current[sc.ID]) == 0 {
				reason := "no finished clip yet"
				for _, j := range jobs {
					if j.SceneID != nil && *j.SceneID == sc.ID && j.Active() {
						reason = "still being made"
					}
				}
				report.Skipped = append(report.Skipped, clipApprovalEntry{SceneID: sc.ID, Label: "“" + sc.Title + "”", Reason: reason})
				continue
			}
			for i := range current[sc.ID] {
				job := current[sc.ID][i]
				outs, _ := s.database.ListOutputsForJob(job.ID) // an unreadable clip is not approved
				for _, out := range outs {
					candidates = append(candidates, candidate{&job, out})
				}
			}
		}
	}
	now := time.Now().UTC()
	for _, c := range candidates {
		scene := ""
		if c.job.SceneID != nil {
			scene = *c.job.SceneID
		}
		entry := clipApprovalEntry{OutputID: c.out.ID, SceneID: scene, Label: clipLabel(titles[scene], c.job)}
		if clipApproved(c.out.QAJSON) {
			report.AlreadyApproved = append(report.AlreadyApproved, entry)
			continue
		}
		if reason := clipApprovalBlock(c.job, &c.out, decisions); reason != "" {
			entry.Reason = reason
			report.Skipped = append(report.Skipped, entry)
			continue
		}
		raw, _ := json.Marshal(clipApproval{Status: "approved", By: by, At: now})
		if e := s.database.SetRenderOutputNote(c.out.ID, "approval", raw); e != nil {
			entry.Reason = "couldn't save the approval: " + e.Error()
			report.Skipped = append(report.Skipped, entry)
			continue
		}
		report.Approved = append(report.Approved, entry)
		if storedSelfCheck(c.out.QAJSON) == nil {
			report.Unchecked = append(report.Unchecked, entry)
		}
	}
	if len(report.Approved) > 0 {
		s.broker.Publish(map[string]any{"type": "clips_approved", "count": len(report.Approved)})
	}
	return report, nil
}

func (s *Server) handleApproveClips(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body approveClipsRequest
	if r.ContentLength != 0 {
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil {
			http.Error(w, "choose the clips to approve", 400)
			return
		}
	}
	report, err := s.approveClips(project, body, approvalByUser)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"approved": report.Approved, "already_approved": report.AlreadyApproved, "skipped": report.Skipped, "unchecked": report.Unchecked, "message": report.message()})
}

// compositionUnapproved lists the scene clips an edit uses that are not approved, in
// the order they play. Uploaded media and exported videos need no approval.
func (s *Server) compositionUnapproved(project string, m composition.Manifest) []clipApprovalEntry {
	titles := map[string]string{}
	if scenes, err := s.database.ListScenes(project); err == nil {
		for _, sc := range scenes {
			titles[sc.ID] = sc.Title
		}
	}
	out, seen := []clipApprovalEntry{}, map[string]bool{}
	for _, c := range m.Clips {
		id, ok := strings.CutPrefix(c.AssetID, "output-")
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		o, err := s.database.GetRenderOutput(id)
		if err != nil {
			continue // a missing source is refused by the export's own check
		}
		if clipApproved(o.QAJSON) {
			continue
		}
		entry := clipApprovalEntry{OutputID: id, Label: "a scene clip"}
		if job, err := s.database.GetRenderJob(o.RenderJobID); err == nil && job.SceneID != nil {
			entry.SceneID, entry.Label = *job.SceneID, clipLabel(titles[*job.SceneID], job)
		}
		out = append(out, entry)
	}
	return out
}

// unapprovedMessage is the export's refusal, naming the clips.
func unapprovedMessage(list []clipApprovalEntry) string {
	names := []string{}
	for _, e := range list {
		names = append(names, e.Label)
	}
	verb := "is"
	if len(list) > 1 {
		verb = "are"
	}
	return fmt.Sprintf("The final video can't be rendered yet: %s %s not approved. Approve the clips that passed their checks (Approve all passed clips), or review the others first.", strings.Join(names, ", "), verb)
}
