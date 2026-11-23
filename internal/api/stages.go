package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// The stage map is the app's own, deterministic answer to "where is this project and
// what comes next". It is built from saved state only (characters, voices, scenes,
// /production/progress, finished clips, saved edits and exports), never from what the
// assistant says, and computing it spends nothing. GET /stages serves it, and a
// compact copy goes into every chat turn so the assistant decides from the same facts.
//
// Stages run in story order. Shots come before scene pictures because a scene's
// storyboard is drawn from its shots and /production/progress checks shots first.

// Stage IDs, in order.
const (
	stageBrief         = "brief"
	stageCharacters    = "characters"
	stageVoices        = "voices"
	stageScript        = "script"
	stageShots         = "shots"
	stageScenePictures = "scene_pictures"
	stageClips         = "clips"
	stageReview        = "review"
	stageEdit          = "edit"
	stageExport        = "export"
)

// stageOrder is every stage, in the order a project moves through them.
var stageOrder = []string{stageBrief, stageCharacters, stageVoices, stageScript, stageShots, stageScenePictures, stageClips, stageReview, stageEdit, stageExport}

// Stage statuses.
const (
	statusDone       = "done"        // nothing left to do here
	statusReady      = "ready"       // the next step can run now
	statusInProgress = "in_progress" // the app is working on it (a video is being made)
	statusBlocked    = "blocked"     // something must be settled first; Missing says what
	statusNotStarted = "not_started" // an earlier stage has to finish first
)

// Cost classes of a step.
const (
	costFree  = "free"
	costImage = "image"
	costVideo = "video"
)

// stageCost is what the next step spends. ImageCalls and VideoSeconds come from the
// same counts the buttons quote (one picture call per character sheet, a scene's
// production plan for new video seconds).
type stageCost struct {
	Class        string `json:"class"`
	ImageCalls   int    `json:"image_calls,omitempty"`
	VideoSeconds int    `json:"video_seconds,omitempty"`
	// Note explains an estimate, for example when picture calls depend on a plan.
	Note string `json:"note,omitempty"`
}

// stageAction is the one recommended step of a stage. Action is a chat action-card
// action (generate_shots, render_scenes, finish_video…) or one of the navigation
// steps open_voices and watch_videos, which only open a page.
type stageAction struct {
	Action       string    `json:"action"`
	Label        string    `json:"label"`
	SceneIDs     []string  `json:"scene_ids,omitempty"`
	CharacterIDs []string  `json:"character_ids,omitempty"`
	EditorPanel  string    `json:"editor_panel,omitempty"`
	Cost         stageCost `json:"cost"`
	// Reversible is false when the step spends credits that cannot be refunded or
	// deletes saved work.
	Reversible bool `json:"reversible"`
}

// projectStage is one stage of the map.
type projectStage struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	// Optional stages never hold the project back: a scene needs no saved voice, and
	// watching the clips before joining them is up to the user.
	Optional bool `json:"optional,omitempty"`
	// Done and Total count the stage's items (characters, scenes) when it has any.
	Done  int `json:"done"`
	Total int `json:"total"`
	// Missing says in plain words what is not there yet, e.g. "no saved voice for Satoshi".
	Missing []string     `json:"missing,omitempty"`
	Next    *stageAction `json:"next,omitempty"`
}

// stageMap is GET /stages.
type stageMap struct {
	// Current is the first required stage that is not done, or "export" when all are.
	Current  string         `json:"current"`
	Complete bool           `json:"complete"`
	Summary  string         `json:"summary"`
	Next     *stageAction   `json:"next,omitempty"`
	Stages   []projectStage `json:"stages"`
}

// stageCharacter and stageScene are the facts the map is computed from.
type stageCharacter struct {
	ID, Name   string
	HasPicture bool
	HasVoice   bool
}

type stageScene struct {
	ID, Title string
	Progress  sceneProgress
	// VideoSeconds is what making this scene's video would generate now.
	VideoSeconds int
}

// stageInputs is everything computeStages reads. Gathering it touches the database
// and storage; computing from it is pure, so tests build it by hand.
type stageInputs struct {
	// BriefDecided is set once the decision log holds a confirmed brief's choices.
	BriefDecided bool
	Characters   []stageCharacter
	Scenes       []stageScene
	// LastClipAt is when the newest finished scene video was saved.
	LastClipAt time.Time
	// LastEditAt and LastExportAt are the newest saved edit and exported video.
	LastEditAt   time.Time
	LastExportAt time.Time
	// Reviewed is set when the decision log says the user watched the clips.
	Reviewed bool
}

var stageLabels = map[string]string{
	stageBrief: "Creative brief", stageCharacters: "Characters", stageVoices: "Voices", stageScript: "Story & scenes",
	stageShots: "Shots", stageScenePictures: "Scene pictures", stageClips: "Scene videos", stageReview: "Review",
	stageEdit: "Edit", stageExport: "Export",
}

// briefDecisionKeys are decision-log keys a confirmed creative brief records.
var briefDecisionKeys = []string{"language", "aspect_ratio", "style_id", "target_duration", "platform"}

// projectStages gathers the project's saved state and computes its stage map.
func (s *Server) projectStages(ctx context.Context, projectID string) (stageMap, error) {
	progress, err := s.projectProgress(ctx, projectID)
	if err != nil {
		return stageMap{}, err
	}
	in, err := s.stageInputs(ctx, projectID, progress)
	if err != nil {
		return stageMap{}, err
	}
	return computeStages(in), nil
}

// stageInputs gathers the facts of the project; progress is its /production/progress,
// passed in so a caller that already has it does not check every scene twice.
func (s *Server) stageInputs(ctx context.Context, projectID string, progress projectProgress) (stageInputs, error) {
	in := stageInputs{}
	chars, err := s.database.ListCharacters(projectID)
	if err != nil {
		return in, err
	}
	samples := s.voiceSamples(projectID)
	for i := range chars {
		c := &chars[i]
		in.Characters = append(in.Characters, stageCharacter{ID: c.ID, Name: c.Name, HasPicture: s.characterHasSheet(c), HasVoice: samples[c.ID] != "" || s.characterHasVoice(projectID, c.ID)})
	}
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		return in, err
	}
	byID := map[string]*models.Scene{}
	for i := range scenes {
		byID[scenes[i].ID] = &scenes[i]
	}
	for _, p := range progress.Scenes {
		scene := byID[p.SceneID]
		if scene == nil {
			continue
		}
		in.Scenes = append(in.Scenes, stageScene{ID: p.SceneID, Title: p.Title, Progress: p, VideoSeconds: s.stageVideoSeconds(ctx, scene, p)})
	}
	jobs, err := s.database.ListRenderJobs(projectID)
	if err != nil {
		return in, err
	}
	for _, j := range jobs {
		if j.Status == models.StatusSucceeded && j.SceneID != nil && j.ShotID == nil && j.UpdatedAt.After(in.LastClipAt) {
			in.LastClipAt = j.UpdatedAt
		}
	}
	in.LastEditAt = s.lastSavedEdit(projectID)
	in.LastExportAt = s.lastExport(projectID)
	decisions, _ := s.database.ListDecisions(projectID) // an unreadable log reads as empty; stages still come from saved work
	for _, d := range decisions {
		for _, key := range briefDecisionKeys {
			in.BriefDecided = in.BriefDecided || d.Key == key
		}
		in.Reviewed = in.Reviewed || d.Key == "reviewed"
	}
	return in, nil
}

// characterHasVoice says whether the character has an approved, usable saved voice.
func (s *Server) characterHasVoice(projectID, characterID string) bool {
	doc, err := s.database.GetVoiceDocument(projectID, "profile", characterID)
	var profile VoiceProfile
	if err != nil || json.Unmarshal(doc.Data, &profile) != nil {
		return false
	}
	return profile.Approved && (voiceChoice{Provider: profile.Provider, VoiceID: profile.VoiceID}).usable()
}

// stageVideoSeconds is the new video seconds making this scene would generate, from the
// scene's production plan (what the render button quotes), else the scene's length.
func (s *Server) stageVideoSeconds(ctx context.Context, scene *models.Scene, p sceneProgress) int {
	if p.Video.State == "ready" || p.Video.State == "making" {
		return 0
	}
	if p.Stage == "ready_to_render" || p.Video.State == "failed" {
		if plan, err := s.sceneProductionPlan(ctx, scene); err == nil {
			return plan.NewSeconds
		}
	}
	return scene.Duration
}

// lastSavedEdit is when the project's newest edit (composition) was saved.
func (s *Server) lastSavedEdit(projectID string) time.Time {
	latest := time.Time{}
	if !composition.ValidID(projectID) {
		return latest
	}
	dir := filepath.Join(s.compositionRoot(), projectID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return latest
	}
	for _, e := range entries {
		if !e.IsDir() || !composition.ValidID(e.Name()) {
			continue
		}
		if info, err := os.Lstat(filepath.Join(dir, e.Name(), "manifest.json")); err == nil && info.Mode().IsRegular() && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// lastExport is when the project's newest exported video (an edit rendered to a file) was made.
func (s *Server) lastExport(projectID string) time.Time {
	latest := time.Time{}
	assets, _ := s.database.ListAssets(projectID) // no readable assets means no export yet
	for _, a := range assets {
		var meta struct {
			Revision string `json:"composition_revision"`
		}
		if a.Type == "output_video" && json.Unmarshal(a.MetadataJSON, &meta) == nil && meta.Revision != "" && a.CreatedAt.After(latest) {
			latest = a.CreatedAt
		}
	}
	return latest
}

// computeStages turns a project's facts into its stage map.
func computeStages(in stageInputs) stageMap {
	stages := []projectStage{
		briefStage(in), charactersStage(in), voicesStage(in), scriptStage(in), shotsStage(in),
		scenePicturesStage(in), clipsStage(in), reviewStage(in), editStage(in), exportStage(in),
	}
	out := stageMap{Stages: stages, Current: stageExport, Complete: true}
	for i := range stages {
		if stages[i].Status != statusDone && !stages[i].Optional {
			out.Current, out.Complete, out.Next = stages[i].ID, false, stages[i].Next
			break
		}
	}
	out.Summary = stageSummary(out)
	return out
}

func newStage(id string) projectStage {
	return projectStage{ID: id, Label: stageLabels[id]}
}

func briefStage(in stageInputs) projectStage {
	st := newStage(stageBrief)
	if in.BriefDecided || len(in.Scenes) > 0 {
		st.Status = statusDone
		return st
	}
	st.Status = statusReady
	st.Missing = []string{"no story idea or creative brief yet"}
	st.Next = &stageAction{Action: "generate_script", Label: "Plan the story", Reversible: true,
		Cost: stageCost{Class: costFree, Note: "planning is free; confirming the plan starts the story, whose pictures use image credits"}}
	return st
}

func charactersStage(in stageInputs) projectStage {
	st := newStage(stageCharacters)
	st.Total = len(in.Characters)
	if len(in.Characters) == 0 {
		// A story can have no characters, and the story run creates the ones it needs:
		// once the brief is settled, writing the story comes first.
		if len(in.Scenes) > 0 || in.BriefDecided {
			st.Status = statusDone
		} else {
			st.Status = statusNotStarted
			st.Missing = []string{"no characters yet; the story run creates them"}
		}
		return st
	}
	without := []stageCharacter{}
	for _, c := range in.Characters {
		if c.HasPicture {
			st.Done++
		} else {
			without = append(without, c)
		}
	}
	if len(without) == 0 {
		st.Status = statusDone
		return st
	}
	st.Status = statusReady
	ids, names := []string{}, []string{}
	for _, c := range without {
		ids, names = append(ids, c.ID), append(names, c.Name)
	}
	st.Missing = []string{countPhrase(len(without), "character has", "characters have") + " no picture: " + strings.Join(names, ", ")}
	st.Next = &stageAction{Action: "generate_character_references", Label: "Make pictures for all characters", CharacterIDs: ids, Reversible: true,
		Cost: stageCost{Class: costImage, ImageCalls: len(without) * characterSheetImageCalls}}
	return st
}

func voicesStage(in stageInputs) projectStage {
	st := newStage(stageVoices)
	st.Optional = true
	st.Total = len(in.Characters)
	missing := []string{}
	for _, c := range in.Characters {
		if c.HasVoice {
			st.Done++
		} else {
			missing = append(missing, "no saved voice for "+c.Name)
		}
	}
	switch {
	case len(in.Characters) == 0 && len(in.Scenes) == 0:
		st.Status = statusNotStarted
	case len(missing) == 0:
		st.Status = statusDone
	default:
		st.Status = statusReady
		st.Missing = missing
		st.Next = &stageAction{Action: "open_voices", Label: "Choose voices", Reversible: true, Cost: stageCost{Class: costFree, Note: "the video model can voice scenes without saved voices"}}
	}
	return st
}

func scriptStage(in stageInputs) projectStage {
	st := newStage(stageScript)
	st.Total, st.Done = len(in.Scenes), len(in.Scenes)
	switch {
	case len(in.Scenes) > 0:
		st.Status = statusDone
	case in.BriefDecided:
		st.Status = statusReady
		st.Missing = []string{"no scenes written yet"}
		st.Next = &stageAction{Action: "generate_script", Label: "Write the story", Reversible: true, Cost: stageCost{Class: costFree}}
	default:
		st.Status = statusNotStarted
		st.Missing = []string{"no scenes written yet"}
	}
	return st
}

func shotsStage(in stageInputs) projectStage {
	st := newStage(stageShots)
	st.Total = len(in.Scenes)
	if len(in.Scenes) == 0 {
		st.Status = statusNotStarted
		return st
	}
	without := []stageScene{}
	for _, sc := range in.Scenes {
		if sc.Progress.Stage == "needs_shots" {
			without = append(without, sc)
		} else {
			st.Done++
		}
	}
	if len(without) == 0 {
		st.Status = statusDone
		return st
	}
	st.Status = statusReady
	st.Missing = []string{countPhrase(len(without), "scene has", "scenes have") + " no shots: " + sceneTitles(without)}
	st.Next = &stageAction{Action: "generate_shots", Label: "Plan shots", SceneIDs: sceneIDs(without[:1]), Reversible: true, Cost: stageCost{Class: costFree}}
	return st
}

// scenePicturesStage covers the pictures a scene's video is made from: its connected
// reference pictures, and its storyboard unless its clips use the pictures directly.
func scenePicturesStage(in stageInputs) projectStage {
	st := newStage(stageScenePictures)
	st.Total = len(in.Scenes)
	if len(in.Scenes) == 0 {
		st.Status = statusNotStarted
		return st
	}
	refs, boards, waiting := []stageScene{}, []stageScene{}, 0
	for _, sc := range in.Scenes {
		switch sc.Progress.Stage {
		case "needs_shots":
			waiting++
		case "needs_references":
			refs = append(refs, sc)
		case "needs_storyboard", "storyboard_outdated":
			boards = append(boards, sc)
		default:
			st.Done++
		}
	}
	switch {
	case len(refs) > 0:
		st.Status = statusReady
		st.Missing = []string{countPhrase(len(refs), "scene is", "scenes are") + " missing reference pictures: " + sceneTitles(refs)}
		st.Next = &stageAction{Action: "generate_assets", Label: "Create missing images", SceneIDs: sceneIDs(refs[:1]), Reversible: true,
			Cost: stageCost{Class: costImage, ImageCalls: len(refs), Note: "at least one picture call per scene; the button shows the exact count"}}
	case len(boards) > 0:
		st.Status = statusReady
		st.Missing = []string{countPhrase(len(boards), "scene needs", "scenes need") + " a storyboard: " + sceneTitles(boards)}
		st.Next = &stageAction{Action: "storyboard", Label: "Make storyboard", SceneIDs: sceneIDs(boards[:1]), Reversible: true,
			Cost: stageCost{Class: costImage, ImageCalls: len(boards)}}
	case waiting > 0:
		st.Status = statusNotStarted
		st.Missing = []string{countPhrase(waiting, "scene needs", "scenes need") + " shots first"}
	default:
		st.Status = statusDone
	}
	return st
}

func clipsStage(in stageInputs) projectStage {
	st := newStage(stageClips)
	st.Total = len(in.Scenes)
	if len(in.Scenes) == 0 {
		st.Status = statusNotStarted
		return st
	}
	ready, making, stuck, notReady := []stageScene{}, 0, []stageScene{}, 0
	for _, sc := range in.Scenes {
		p := sc.Progress
		switch {
		case p.Video.State == "ready":
			st.Done++
		case p.Video.State == "making":
			making++
		case p.Video.State == "waiting_on_service":
			stuck = append(stuck, sc)
		case p.Stage == "ready_to_render" || p.Video.State == "failed" && p.Stage == "needs_attention":
			ready = append(ready, sc)
		default:
			notReady++
		}
	}
	switch {
	case st.Done == st.Total:
		st.Status = statusDone
	case len(stuck) > 0:
		st.Status = statusBlocked
		st.Missing = []string{countPhrase(len(stuck), "scene is", "scenes are") + " waiting on an answer from the video service: " + sceneTitles(stuck)}
		st.Next = &stageAction{Action: "check_video", Label: "Check again", SceneIDs: sceneIDs(stuck[:1]), Reversible: true, Cost: stageCost{Class: costFree}}
	case len(ready) > 0:
		st.Status = statusReady
		seconds := 0
		for _, sc := range ready {
			seconds += sc.VideoSeconds
		}
		st.Missing = []string{countPhrase(len(ready), "scene has", "scenes have") + " no video yet: " + sceneTitles(ready)}
		action, label := "render_scenes", "Make these videos"
		if len(ready) == 1 {
			action, label = "render_scene", "Make video"
		}
		st.Next = &stageAction{Action: action, Label: label, SceneIDs: sceneIDs(ready), Reversible: false, Cost: stageCost{Class: costVideo, VideoSeconds: seconds}}
	case making > 0:
		st.Status = statusInProgress
		st.Missing = []string{countPhrase(making, "scene video is", "scene videos are") + " being made"}
	default:
		st.Status = statusNotStarted
		st.Missing = []string{countPhrase(notReady, "scene needs", "scenes need") + " shots and pictures first"}
	}
	return st
}

func allClipsDone(in stageInputs) bool {
	if len(in.Scenes) == 0 {
		return false
	}
	for _, sc := range in.Scenes {
		if sc.Progress.Video.State != "ready" {
			return false
		}
	}
	return true
}

// editIsCurrent says whether the newest saved edit was made after the newest clip.
func editIsCurrent(in stageInputs) bool {
	return !in.LastEditAt.IsZero() && !in.LastEditAt.Before(in.LastClipAt)
}

func reviewStage(in stageInputs) projectStage {
	st := newStage(stageReview)
	st.Optional = true
	switch {
	case !allClipsDone(in):
		st.Status = statusNotStarted
	case in.Reviewed || editIsCurrent(in):
		st.Status = statusDone
	default:
		st.Status = statusReady
		st.Next = &stageAction{Action: "watch_videos", Label: "Watch the scene videos", Reversible: true, Cost: stageCost{Class: costFree}}
	}
	return st
}

func editStage(in stageInputs) projectStage {
	st := newStage(stageEdit)
	switch {
	case !allClipsDone(in):
		st.Status = statusNotStarted
	case editIsCurrent(in):
		st.Status = statusDone
	default:
		st.Status = statusReady
		st.Missing = []string{"the scene videos are not joined into one video yet"}
		st.Next = &stageAction{Action: "finish_video", Label: "Join the scenes and export", Reversible: true,
			Cost: stageCost{Class: costFree, Note: "every scene already has its video, so joining, captions and export spend nothing"}}
	}
	return st
}

func exportStage(in stageInputs) projectStage {
	st := newStage(stageExport)
	switch {
	case !allClipsDone(in) || !editIsCurrent(in):
		st.Status = statusNotStarted
	case !in.LastExportAt.IsZero() && !in.LastExportAt.Before(in.LastEditAt):
		st.Status = statusDone
	default:
		st.Status = statusReady
		st.Missing = []string{"the latest edit has not been exported"}
		st.Next = &stageAction{Action: "open_editor", Label: "Export the video", EditorPanel: "export", Reversible: true, Cost: stageCost{Class: costFree}}
	}
	return st
}

func stageSummary(m stageMap) string {
	if m.Complete {
		return "Every stage is done: the final video is exported."
	}
	for _, st := range m.Stages {
		if st.ID != m.Current {
			continue
		}
		text := st.Label + ": " + strings.ReplaceAll(st.Status, "_", " ")
		if len(st.Missing) > 0 {
			text += " — " + st.Missing[0]
		}
		if m.Next != nil {
			text += ". Next: " + m.Next.Label
		}
		return text + "."
	}
	return ""
}

func countPhrase(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func sceneIDs(scenes []stageScene) []string {
	ids := []string{}
	for _, sc := range scenes {
		ids = append(ids, sc.ID)
	}
	return ids
}

// sceneTitles names up to four scenes, in quotes.
func sceneTitles(scenes []stageScene) string {
	names := []string{}
	for i, sc := range scenes {
		if i == 4 {
			names = append(names, fmt.Sprintf("and %d more", len(scenes)-4))
			break
		}
		names = append(names, "“"+sc.Title+"”")
	}
	return strings.Join(names, ", ")
}

// chatStages is the compact stage map each chat turn carries: the current stage, its
// next step, and one line per stage.
func chatStages(m stageMap) map[string]any {
	lines := []string{}
	for _, st := range m.Stages {
		line := st.ID + ": " + st.Status
		if st.Optional {
			line += " (optional)"
		}
		if st.Total > 0 {
			line += fmt.Sprintf(" %d/%d", st.Done, st.Total)
		}
		if len(st.Missing) > 0 {
			line += " — " + strings.Join(st.Missing, "; ")
		}
		lines = append(lines, line)
	}
	return map[string]any{"current": m.Current, "complete": m.Complete, "summary": m.Summary, "next": m.Next, "stages": lines}
}

func (s *Server) handleStages(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	out, err := s.projectStages(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
