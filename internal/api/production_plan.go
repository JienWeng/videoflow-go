package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"time"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

type productionSettings struct {
	DialogueShots map[string]string `json:"dialogue_shots,omitempty"`
	AudioMode     string            `json:"audio_mode"`
	Strategy      string            `json:"strategy"`
	MusicPrompt   string            `json:"music_prompt"`
}
type renderUnit struct {
	Count    int      `json:"count"`
	Index    int      `json:"index"`
	ShotIDs  []string `json:"shot_ids"`
	Start    int      `json:"start"`
	Length   int      `json:"length"`
	Duration int      `json:"duration"`
	Prompt   string   `json:"prompt"`
	Key      string   `json:"key"`
	JobID    string   `json:"job_id,omitempty"`
	Status   string   `json:"status,omitempty"`
}
type productionLine struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Speaker string `json:"speaker"`
	ShotID  string `json:"shot_id"`
}
type productionPlan struct {
	Resolution       string             `json:"resolution"`
	Lines            []productionLine   `json:"lines"`
	ID               string             `json:"id"`
	SceneID          string             `json:"scene_id"`
	UpdatedAt        time.Time          `json:"updated_at"`
	Settings         productionSettings `json:"settings"`
	Provider         string             `json:"provider"`
	Model            string             `json:"model"`
	AudioSupported   bool               `json:"audio_supported"`
	Units            []renderUnit       `json:"units"`
	Calls            int                `json:"calls"`
	Reused           int                `json:"reused"`
	GeneratedSeconds int                `json:"generated_seconds"`
	NewSeconds       int                `json:"new_seconds"`
	EditedSeconds    int                `json:"edited_seconds"`
	StoryboardReady  bool               `json:"storyboard_ready"`
	ReferencesReady  bool               `json:"references_ready"`
	Blocked          string             `json:"blocked,omitempty"`
}

func sceneProduction(scene *models.Scene) productionSettings {
	var d struct {
		Production productionSettings `json:"production"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &d)
	if d.Production.Strategy == "" {
		d.Production.Strategy = "auto"
	}
	return d.Production
}
func validateProduction(p productionSettings) error {
	if p.AudioMode != "" && p.AudioMode != "native" && p.AudioMode != "saved" && p.AudioMode != "silent" {
		return fmt.Errorf("choose native, saved or silent audio")
	}
	if p.Strategy != "auto" && p.Strategy != "shots" {
		return fmt.Errorf("choose automatic grouping or separate shots")
	}
	if len(p.MusicPrompt) > 1500 {
		return fmt.Errorf("music direction must be at most 1500 characters")
	}
	if p.AudioMode != "native" && strings.TrimSpace(p.MusicPrompt) != "" {
		return fmt.Errorf("native music direction requires native audio; add a separate soundtrack in the editor for saved or silent mode")
	}
	return nil
}
func (s *Server) productionVideoModel(ctx context.Context) (string, *providers.Model, error) {
	provider, model := s.routes().MediaSelection("video")
	// These are the same documented capabilities used by the existing adapter.
	if (provider == "fal" && (model == providers.FalVideoModel || model == "bytedance/seedance-2.0/fast/text-to-video")) || (provider == "byteplus" && model == providers.BytePlusVideoModel) {
		m := &providers.Model{ID: model, GenerateAudio: true, Resolutions: []string{"480p", "720p"}}
		for i := 4; i <= 15; i++ {
			m.Durations = append(m.Durations, i)
		}
		return provider, m, nil
	}
	client, _, err := s.routes().Media(provider, model, "")
	if err != nil {
		return provider, nil, err
	}
	m, err := client.Model(ctx, model, "video")
	return provider, m, err
}
func groupRenderUnits(shots []models.Shot, total int, strategy string, durations []int) ([]renderUnit, error) {
	if len(shots) == 0 {
		return nil, fmt.Errorf("plan or add shots before rendering")
	}
	max := 0
	for _, d := range durations {
		if d > max {
			max = d
		}
	}
	if max == 0 {
		return nil, fmt.Errorf("selected model has no verified clip durations")
	}
	sum := 0
	for _, shot := range shots {
		if shot.Duration <= 0 || shot.Duration > max {
			return nil, fmt.Errorf("shot %d duration must be between 1 and %d seconds; replan or split this shot before rendering", shot.ShotOrder, max)
		}
		sum += shot.Duration
	}
	if sum != total {
		return nil, fmt.Errorf("shot timings total %ds but scene is %ds; adjust shot timings before rendering", sum, total)
	}
	// Minimize paid calls first, then billed seconds. A greedy full first clip can
	// leave a one-second tail that still costs a four-second minimum generation.
	type choice struct{ calls, seconds, end, duration int }
	best := make([]choice, len(shots)+1)
	for i := len(shots) - 1; i >= 0; i-- {
		best[i].calls = len(shots) + 1
		length := 0
		for j := i; j < len(shots); j++ {
			if strategy == "shots" && j > i {
				break
			}
			length += shots[j].Duration
			if length > max {
				break
			}
			duration := 0
			for _, d := range durations {
				if d >= length && (duration == 0 || d < duration) {
					duration = d
				}
			}
			if duration == 0 {
				continue
			}
			calls, seconds := 1+best[j+1].calls, duration+best[j+1].seconds
			if calls < best[i].calls || (calls == best[i].calls && seconds <= best[i].seconds) {
				best[i] = choice{calls, seconds, j + 1, duration}
			}
		}
	}
	units := []renderUnit{}
	start := 0
	for i := 0; i < len(shots); {
		selected := best[i]
		if selected.end <= i {
			return nil, fmt.Errorf("clip duration cannot be supported")
		}
		unit := renderUnit{Index: len(units), Start: start, Duration: selected.duration, ShotIDs: []string{}}
		for _, shot := range shots[i:selected.end] {
			unit.ShotIDs = append(unit.ShotIDs, shot.ID)
			unit.Prompt += fmt.Sprintf("\n%ds–%ds, shot %d: %s", unit.Length, unit.Length+shot.Duration, shot.ShotOrder, shot.Prompt)
			if shot.Camera != nil {
				unit.Prompt += " Camera: " + *shot.Camera
			}
			if shot.Movement != nil {
				unit.Prompt += " Movement: " + *shot.Movement
			}
			unit.Length += shot.Duration
		}
		units = append(units, unit)
		start += unit.Length
		i = selected.end
	}

	for i := range units {
		units[i].Count = len(units)
	}
	return units, nil
}
func (s *Server) sceneProductionPlan(ctx context.Context, scene *models.Scene) (productionPlan, error) {
	p := productionPlan{SceneID: scene.ID, UpdatedAt: scene.UpdatedAt, Settings: sceneProduction(scene), EditedSeconds: scene.Duration}
	provider, model, err := s.productionVideoModel(ctx)
	if err != nil {
		return p, err
	}
	p.Provider, p.Model, p.AudioSupported = provider, model.ID, model.GenerateAudio
	if provider == "fal" && model.ID == providers.FalNativeVoiceModel {
		return p, fmt.Errorf("reference-to-video uses explicit media roles; use Generate from references in the editing workspace, or choose an image-to-video model for scene rendering")
	}
	p.Resolution = sceneBrief(scene).Resolution
	if p.Resolution == "" {
		p.Resolution = "auto"
	}
	if p.Resolution != "auto" {
		supported := false
		for _, r := range model.Resolutions {
			supported = supported || r == p.Resolution
		}
		if !supported {
			return p, fmt.Errorf("selected model does not support the requested %s size; choose a compatible model or model-default resolution", p.Resolution)
		}
	}

	if p.Settings.AudioMode == "" {
		p.Settings.AudioMode = "saved"
		if model.GenerateAudio {
			p.Settings.AudioMode = "native"
		}
		if sceneBrief(scene).ConversationMode == "silent" {
			p.Settings.AudioMode = "silent"
		}
	}
	if err = validateProduction(p.Settings); err != nil {
		return p, err
	}
	if p.Settings.AudioMode == "native" && !model.GenerateAudio {
		return p, fmt.Errorf("selected model does not support native audio; choose saved voices or silent")
	}
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return p, err
	}
	p.Units, err = groupRenderUnits(shots, scene.Duration, p.Settings.Strategy, model.Durations)
	if err != nil {
		return p, err
	}
	refs, err := s.sceneReferenceState(ctx, scene)
	if err != nil {
		return p, err
	}
	p.StoryboardReady = refs["storyboard_ready"] == true
	p.ReferencesReady = refs["references_ready"] == true
	identity := map[string]any{"scene_id": scene.ID, "project": scene.ProjectID, "scene_revision": refs["scene_input_revision"], "identity_revision": refs["context_revision"], "settings": p.Settings, "resolution": p.Resolution, "provider": provider, "model": model.ID, "units": p.Units}
	if p.Resolution == "auto" {
		delete(identity, "resolution")
	} // Preserve existing default-resolution plan IDs.
	raw := voiceJSON(identity)
	hash := sha256.Sum256(raw)
	p.ID = hex.EncodeToString(hash[:])
	jobs, err := s.database.ListRenderJobs(*scene.ProjectID)
	if err != nil {
		return p, err
	}
	for i := range p.Units {
		u := &p.Units[i]
		u.Key = fmt.Sprintf("%s:%d", p.ID, i)
		p.GeneratedSeconds += u.Duration
		for _, job := range jobs {
			if job.SceneID == nil || *job.SceneID != scene.ID {
				continue
			}
			var r struct {
				Unit renderUnit `json:"render_unit"`
			}
			_ = json.Unmarshal(job.RequestJSON, &r)
			active := job.Status == "pending" || job.Status == "running" || job.Status == "reconciliation_required"
			if r.Unit.Key == u.Key && (active || job.Status == "succeeded") && u.JobID == "" {
				u.JobID, u.Status = job.ID, job.Status
			}
			if active && (r.Unit.Key == "" || !strings.HasPrefix(r.Unit.Key, p.ID+":")) {
				p.Blocked = "Another or unresolved render is active for this scene. Check Videos before starting a different plan."
			}
			if active && job.Status == "reconciliation_required" {
				p.Blocked = "A render has an uncertain provider outcome. Resolve it in Videos before spending again."
			}
		}
		if u.JobID != "" {
			p.Reused++
		} else {
			p.Calls++
			p.NewSeconds += u.Duration
		}
	}
	// A multi-clip native render needs explicit dialogue allocation, not repeated lines in every clip.
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script)
	for i, line := range script.Lines {
		if strings.TrimSpace(line.Text) == "" {
			continue
		}
		id := line.ID
		if id == "" {
			id = fmt.Sprintf("line-%d", i)
		}
		assigned := p.Settings.DialogueShots[id]
		valid := false
		for _, u := range p.Units {
			for _, shot := range u.ShotIDs {
				if shot == assigned {
					valid = true
				}
			}
		}
		p.Lines = append(p.Lines, productionLine{ID: id, Text: line.Text, Speaker: line.CharacterID, ShotID: assigned})
		if p.Settings.AudioMode == "native" && len(p.Units) > 1 && !valid {
			p.Blocked = "Assign each spoken line to a clip below before generating multiple clips with native dialogue."
		}
	}
	return p, nil
}
func (s *Server) productionScene(w http.ResponseWriter, r *http.Request) (*models.Scene, bool) {
	project, err := s.activeProjectID(r.Context())
	scene, e := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene is not in the active project", 404)
		return nil, false
	}
	return scene, true
}
func (s *Server) handleProductionPlan(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.productionScene(w, r)
	if !ok {
		return
	}
	if r.Method == "PUT" {
		var body struct {
			UpdatedAt time.Time          `json:"updated_at"`
			Settings  productionSettings `json:"settings"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !body.UpdatedAt.Equal(scene.UpdatedAt) {
			http.Error(w, "scene changed; reload the production plan before saving", 409)
			return
		}
		if err := validateProduction(body.Settings); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var doc map[string]any
		if json.Unmarshal(scene.SceneJSON, &doc) != nil || doc == nil {
			doc = map[string]any{}
		}
		doc["production"] = body.Settings
		scene.SceneJSON = voiceJSON(doc)
		// Resolve capabilities and validate the plan before persisting an unusable choice.
		if _, err := s.sceneProductionPlan(r.Context(), scene); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		if err := s.database.UpdateScene(scene); err != nil {
			http.Error(w, "scene changed; reload before saving", 409)
			return
		}
	}
	plan, err := s.sceneProductionPlan(r.Context(), scene)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	writeJSON(w, 200, plan)
}
