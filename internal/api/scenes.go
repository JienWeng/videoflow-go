package api

import (
	"encoding/json"
	"net/http"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListScenes(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scenes == nil {
		scenes = []models.Scene{}
	}
	writeJSON(w, http.StatusOK, scenes)
}

func (s *Server) handleGetScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(id)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleCreateScene(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var scene models.Scene
	if err := json.NewDecoder(r.Body).Decode(&scene); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	scene.ProjectID = &projectID
	if scene.Duration <= 0 {
		scene.Duration = 5
	}
	if scene.AspectRatio == "" {
		scene.AspectRatio = "16:9"
	}
	if err := s.database.CreateScene(&scene); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, scene)
}

func (s *Server) handleUpdateScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(id)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(scene); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	scene.ID = id
	if err := s.database.UpdateScene(scene); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleDeleteScene(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteScene(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	shots, err := s.database.ListShots(sceneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if shots == nil {
		shots = []models.Shot{}
	}
	writeJSON(w, http.StatusOK, shots)
}

func (s *Server) handleCreateShot(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	var shot models.Shot
	if err := json.NewDecoder(r.Body).Decode(&shot); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	shot.SceneID = sceneID
	if shot.Duration <= 0 {
		shot.Duration = 5
	}
	if err := s.database.CreateShot(&shot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, shot)
}

func (s *Server) handleUpdateShot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	shot, err := s.database.GetShot(id)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(shot); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	shot.ID = id
	if err := s.database.UpdateShot(shot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleDeleteShot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteShot(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleExpandScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	shots, err := s.agents.GenerateShots(r.Context(), scene.Title, scene.Summary, scene.Duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	createdShots := []models.Shot{}
	for _, sh := range shots {
		shot := models.Shot{
			SceneID:   scene.ID,
			ShotOrder: sh.Order,
			Duration:  sh.Duration,
			Prompt:    sh.Prompt,
			Camera:    &sh.Camera,
			Movement:  &sh.Movement,
		}
		_ = s.database.CreateShot(&shot)
		createdShots = append(createdShots, shot)
	}
	writeJSON(w, http.StatusOK, createdShots)
}

func (s *Server) handleRefineScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleDevelopIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Idea string `json:"idea"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	res, err := s.agents.DevelopIdea(r.Context(), body.Idea)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGenerateScript(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body struct {
		Idea           string `json:"idea"`
		TargetDuration int    `json:"target_duration"`
		SceneCount     int    `json:"scene_count"`
		AspectRatio    string `json:"aspect_ratio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	draft, err := s.agents.GenerateScript(r.Context(), body.Idea, body.SceneCount, body.TargetDuration, body.AspectRatio)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	draftBytes, _ := json.Marshal(draft)
	script := &models.Script{
		ProjectID: &projectID,
		Idea:      body.Idea,
		Title:     draft.Title,
		Summary:   draft.Summary,
		DraftJSON: draftBytes,
	}
	_ = s.database.CreateScript(script)

	createdScenes := []models.Scene{}
	for i, sc := range draft.Scenes {
		scene := models.Scene{
			ProjectID:   &projectID,
			ScriptID:    &script.ID,
			SceneOrder:  i + 1,
			Title:       sc.Title,
			Summary:     sc.Summary,
			Duration:    sc.Duration,
			AspectRatio: sc.AspectRatio,
		}
		_ = s.database.CreateScene(&scene)
		createdScenes = append(createdScenes, scene)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"script": script,
		"draft":  draft,
		"scenes": createdScenes,
	})
}

func (s *Server) handleListScripts(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	scripts, err := s.database.ListScripts(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scripts == nil {
		scripts = []models.Script{}
	}
	writeJSON(w, http.StatusOK, scripts)
}
