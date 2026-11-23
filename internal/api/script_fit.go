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
	"github.com/google/uuid"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// Fit script to clips: the local AI session rewrites only a scene's spoken lines, in one
// turn and without credits, so each shot's lines fill most of its seconds and never
// more, and each line is saved with its shot. The proposal is shown before and after;
// in Manual and Auto it is saved only when the owner applies it, and Full auto applies
// it by itself before a scene's video is made (autopilot.go).

type scriptFit struct {
	SceneID string `json:"scene_id"`
	// ExpectedUpdatedAt is the scene version the proposal was made from; applying it
	// with PUT /scenes/{id}/spoken-lines refuses a scene changed since.
	ExpectedUpdatedAt time.Time    `json:"expected_updated_at"`
	Before            []SpokenLine `json:"before"`
	After             []SpokenLine `json:"after"`
	BeforeFit         sceneSpeech  `json:"before_fit"`
	AfterFit          sceneSpeech  `json:"after_fit"`
}

// speechNeedsFit says whether a scene's speech should be fitted: something cannot be
// said in its clip, or a line runs past the shot it is spoken in.
func speechNeedsFit(plan sceneSpeech) bool {
	if plan.Verdict == speech.TooLong {
		return true
	}
	for _, sh := range plan.Shots {
		if sh.Verdict == speech.TooLong {
			return true
		}
	}
	return false
}

// proposeScriptFit asks the local AI session once for the scene's lines fitted to its
// shots, checks them, and returns them beside the lines as they are. Nothing is saved.
func (s *Server) proposeScriptFit(ctx context.Context, scene *models.Scene) (*scriptFit, error) {
	before := sceneSpokenLines(scene)
	if len(before) == 0 {
		return nil, fmt.Errorf("“%s” has no spoken lines to fit", scene.Title)
	}
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(shots, func(i, j int) bool { return shots[i].ShotOrder < shots[j].ShotOrder })
	plan := s.sceneSpeechFor(ctx, scene, before)
	project := *scene.ProjectID
	names := s.speakerNames(project)

	fitShots := []agents.FitShot{}
	number := map[string]int{}
	for _, sh := range plan.Shots {
		n := max(1, sh.Number)
		number[sh.ShotID] = n
		action := scene.Summary
		for _, m := range shots {
			if m.ID == sh.ShotID {
				action = m.Prompt
			}
		}
		b := sh.Budget
		fitShots = append(fitShots, agents.FitShot{Number: n, Seconds: int(sh.Seconds), Action: action, Target: fmt.Sprintf("%d-%d %s", b.Low, b.High, b.Unit), Most: fmt.Sprintf("%d %s", b.Max, b.Unit)})
	}
	speakers := map[string]string{"": "Narrator"}
	for _, id := range models.IDList(scene.CharacterIDsJSON) {
		if names[id] != "" {
			speakers[id] = names[id]
		}
	}
	fitLines := []agents.FitLine{}
	for _, sh := range plan.Shots {
		for _, l := range sh.Lines {
			speakers[l.CharacterID] = l.Speaker
			fitLines = append(fitLines, agents.FitLine{ID: l.ID, CharacterID: l.CharacterID, Speaker: l.Speaker, Text: l.Text, Shot: number[sh.ShotID], Seconds: l.Seconds})
		}
	}
	language := plan.Language
	if language == "" {
		language = before[0].Language
	}
	fitted, err := s.agents.FitScript(ctx, scene.Title, scene.Summary, language, fitShots, fitLines, speakers)
	if err != nil {
		return nil, fmt.Errorf("couldn't fit the script of “%s”: %w", scene.Title, err)
	}

	// Each rewritten line keeps the language and on-screen flag of the line it came from
	// (or of its speaker's other lines), and is saved with its shot.
	byID, bySpeaker := map[string]SpokenLine{}, map[string]SpokenLine{}
	for _, l := range before {
		byID[l.ID] = l
		if _, ok := bySpeaker[l.CharacterID]; !ok {
			bySpeaker[l.CharacterID] = l
		}
	}
	after := []SpokenLine{}
	for _, f := range fitted {
		from, ok := byID[f.ID]
		if !ok {
			from, ok = byID[strings.TrimSuffix(f.ID, "_b")]
		}
		if !ok || from.CharacterID != f.CharacterID {
			from, ok = bySpeaker[f.CharacterID]
		}
		line := SpokenLine{ID: f.ID, CharacterID: f.CharacterID, Language: from.Language, Text: strings.TrimSpace(f.Text), Visible: from.Visible && f.CharacterID != ""}
		if !ok {
			line.Language, line.Visible = language, f.CharacterID != ""
		}
		if line.Language == "" {
			line.Language = language
		}
		if len(shots) > 0 && f.Shot >= 1 && f.Shot <= len(shots) {
			line.ShotID = shots[f.Shot-1].ID
		}
		after = append(after, line)
	}
	after, err = s.validSpokenLines(project, after, shots, scene.Duration)
	if err != nil {
		return nil, fmt.Errorf("the fitted script of “%s” can't be used: %w", scene.Title, err)
	}
	return &scriptFit{SceneID: scene.ID, ExpectedUpdatedAt: scene.UpdatedAt, Before: before, After: after, BeforeFit: plan, AfterFit: s.sceneSpeechFor(ctx, scene, after)}, nil
}

// fitSceneScript fits a scene's script and saves it, for Full auto and a new story. A
// rewrite that still has a clip too long to say is saved only when it is shorter than
// what it replaces; the caller checks the scene again. It never asks twice.
func (s *Server) fitSceneScript(ctx context.Context, scene *models.Scene) (sceneSpeech, error) {
	fit, err := s.proposeScriptFit(ctx, scene)
	if err != nil {
		return sceneSpeech{}, err
	}
	worse := fit.AfterFit.Verdict == speech.TooLong && longestClip(fit.AfterFit) >= longestClip(fit.BeforeFit)
	if worse {
		return fit.BeforeFit, nil
	}
	if err = s.saveSpokenLines(scene, fit.After); err != nil {
		return sceneSpeech{}, err
	}
	return fit.AfterFit, nil
}

// longestClip is how far past its end the most overfull clip runs (negative when every
// clip has room).
func longestClip(plan sceneSpeech) float64 {
	worst := -1e9
	for _, c := range plan.Clips {
		if c.Used > 0 {
			worst = max(worst, c.Used-c.Seconds)
		}
	}
	return worst
}

// handleSceneScriptFit is POST /scenes/{id}/spoken-lines/fit: the proposal, not saved.
func (s *Server) handleSceneScriptFit(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, sceneErr := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || sceneErr != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found in active project", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	fit, err := s.proposeScriptFit(ctx, scene)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, fit)
}

// shotLines are a scene's spoken lines as the shot planner is given them: who says
// what, and how long it takes to say.
func (s *Server) shotLines(scene *models.Scene) []agents.ShotLine {
	project := ""
	if scene.ProjectID != nil {
		project = *scene.ProjectID
	}
	names := s.speakerNames(project)
	language := sceneBrief(scene).Language
	out := []agents.ShotLine{}
	for _, l := range sceneSpokenLines(scene) {
		if strings.TrimSpace(l.Text) == "" || l.ID == "" {
			continue
		}
		who := names[l.CharacterID]
		if l.CharacterID == "" {
			who = "Narrator"
		}
		lang := l.Language
		if lang == "" {
			lang = language
		}
		out = append(out, agents.ShotLine{ID: l.ID, Speaker: who, Text: l.Text, Seconds: speech.Round(speech.Seconds(l.Text, lang, s.speechCalibration()))})
	}
	return out
}

// placeDraftLines writes on the scene's lines the shot each was planned in, giving the
// new shots their ids now so the shots and the lines are saved together. The scene's old
// shots are being replaced, so a line the planner left out loses its old shot and start
// and is placed by time.
func placeDraftLines(scene *models.Scene, shots []models.Shot, drafts []agents.ShotDraft) {
	lines := sceneSpokenLines(scene)
	if len(lines) == 0 {
		return
	}
	shotOf := map[string]string{}
	for i := range shots {
		if shots[i].ID == "" {
			shots[i].ID = "shot_" + uuid.New().String()[:8]
		}
		if i < len(drafts) {
			for _, id := range drafts[i].LineIDs {
				shotOf[id] = shots[i].ID
			}
		}
	}
	for i := range lines {
		lines[i].ShotID, lines[i].StartHint = shotOf[lines[i].ID], nil
	}
	var snapshot map[string]json.RawMessage
	if json.Unmarshal(scene.SceneJSON, &snapshot) != nil || snapshot == nil {
		return // unreadable scene metadata keeps its lines as they were
	}
	snapshot["spoken_lines"], _ = json.Marshal(lines)
	scene.SceneJSON, _ = json.Marshal(snapshot)
}
