package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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

	// Snapshot revision before update
	oldSnap, _ := json.Marshal(scene)
	_, _ = s.database.CreateRevision("scene", id, oldSnap)

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
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": id, "shots_deleted": 0})
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

	// Snapshot revision before update
	oldSnap, _ := json.Marshal(shot)
	_, _ = s.database.CreateRevision("shot", id, oldSnap)

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
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
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

func (s *Server) handleGenerateShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"

	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()

	if background {
		op := &models.Op{
			Kind:      "shots",
			Status:    "running",
			SceneID:   &sceneID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID string, sc models.Scene) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			shots, err := s.agents.GenerateShots(ctx, sc.Title, sc.Summary, sc.Duration)
			if err != nil {
				errMsg := err.Error()
				_ = s.database.UpdateOp(opID, "failed", nil, &errMsg)
				s.broker.Publish(map[string]interface{}{
					"type":     "op_failed",
					"op_id":    opID,
					"kind":     "shots",
					"status":   "failed",
					"scene_id": sc.ID,
					"error":    errMsg,
				})
				return
			}

			createdShots := []models.Shot{}
			for _, sh := range shots {
				shot := models.Shot{
					SceneID:   sc.ID,
					ShotOrder: sh.Order,
					Duration:  sh.Duration,
					Prompt:    sh.Prompt,
					Camera:    &sh.Camera,
					Movement:  &sh.Movement,
				}
				_ = s.database.CreateShot(&shot)
				createdShots = append(createdShots, shot)
			}

			resBytes, _ := json.Marshal(createdShots)
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":     "op_done",
				"op_id":    opID,
				"kind":     "shots",
				"status":   "succeeded",
				"scene_id": sc.ID,
			})
		}(op.ID, *scene)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
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

func (s *Server) handleReorderShots(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	var body struct {
		OrderedIDs []string `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.database.ReorderShots(sceneID, body.OrderedIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	shots, _ := s.database.ListShots(sceneID)
	if shots == nil {
		shots = []models.Shot{}
	}
	writeJSON(w, http.StatusOK, shots)
}

func (s *Server) handleRefineScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	var body struct {
		Instruction string `json:"instruction"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Instruction != "" {
		scene.Summary = scene.Summary + " (Refined: " + body.Instruction + ")"
		_ = s.database.UpdateScene(scene)
	}
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleRefineShot(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	shot, err := s.database.GetShot(shotID)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}

	var body struct {
		Instruction string `json:"instruction"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Instruction != "" {
		shot.Prompt = shot.Prompt + ", " + body.Instruction
		_ = s.database.UpdateShot(shot)
	}
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleRenderScene(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	stage := "submitted"
	progress := "Whole scene render queued"
	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &scene.ID,
		Provider:    s.cfg.DefaultVideoProvider,
		Model:       s.cfg.OpenRouterVideoModel,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: []byte(fmt.Sprintf(`{"scene_id":"%s"}`, scene.ID)),
	}
	_ = s.database.CreateRenderJob(job)
	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleStoryboard(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	if background {
		op := &models.Op{
			Kind:      "storyboard",
			Status:    "running",
			SceneID:   &sceneID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, scID string) {
			time.Sleep(1 * time.Second)
			resBytes, _ := json.Marshal(map[string]string{
				"scene_id":       scID,
				"storyboard_url": "/storage/storyboards/sample.png",
			})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":     "op_done",
				"op_id":    opID,
				"kind":     "storyboard",
				"status":   "succeeded",
				"scene_id": scID,
			})
		}(op.ID, sceneID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"scene_id":       sceneID,
		"storyboard_url": "/storage/storyboards/sample.png",
	})
}

func (s *Server) handlePlanSceneAssets(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		http.Error(w, "scene not found", http.StatusNotFound)
		return
	}

	var body struct {
		Instruction string `json:"instruction"`
		MaxAssets   int    `json:"max_assets"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	plan := map[string]interface{}{
		"scene_id": scene.ID,
		"assets": []map[string]interface{}{
			{
				"name":        "Key Prop",
				"type":        "prop",
				"description": "Essential prop for " + scene.Title,
			},
		},
		"suggested_shots": []string{},
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleGenerateSceneAssets(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	if background {
		op := &models.Op{
			Kind:      "assets",
			Status:    "running",
			SceneID:   &sceneID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, scID, pid string) {
			time.Sleep(1 * time.Second)
			assetID := "asset_" + uuid.New().String()[:8]
			newAsset := &models.Asset{
				ID:           assetID,
				ProjectID:    &pid,
				Type:         "prop",
				Name:         "Scene Asset",
				FilePath:     "storage/assets/default_prop.png",
				TagsJSON:     []byte("[]"),
				MetadataJSON: []byte(fmt.Sprintf(`{"scene_id":"%s"}`, scID)),
			}
			_ = s.database.CreateAsset(newAsset)

			resBytes, _ := json.Marshal([]models.Asset{*newAsset})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":     "op_done",
				"op_id":    opID,
				"kind":     "assets",
				"status":   "succeeded",
				"scene_id": scID,
			})
		}(op.ID, sceneID, projectID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, []map[string]string{
		{"name": "Scene Asset", "status": "planned"},
	})
}

func (s *Server) handleCastMemberAdd(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	if err := s.database.AddSceneCast(sceneID, characterID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scene, _ := s.database.GetScene(sceneID)
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleCastMemberDelete(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	characterID := chi.URLParam(r, "character_id")
	if err := s.database.RemoveSceneCast(sceneID, characterID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scene, _ := s.database.GetScene(sceneID)
	writeJSON(w, http.StatusOK, scene)
}

func (s *Server) handleShotAssetAttach(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "asset_id")
	if err := s.database.AttachShotAsset(shotID, assetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shot, _ := s.database.GetShot(shotID)
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleShotAssetDetach(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	assetID := chi.URLParam(r, "asset_id")
	if err := s.database.DetachShotAsset(shotID, assetID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shot, _ := s.database.GetShot(shotID)
	writeJSON(w, http.StatusOK, shot)
}

func (s *Server) handleConversationalize(w http.ResponseWriter, r *http.Request) {
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	if background {
		op := &models.Op{
			Kind:      "conversation",
			Status:    "running",
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID string) {
			time.Sleep(1 * time.Second)
			resBytes, _ := json.Marshal(map[string]string{
				"status":  "converted",
				"message": "Conversational conversion complete",
			})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":   "op_done",
				"op_id":  opID,
				"kind":   "conversation",
				"status": "succeeded",
			})
		}(op.ID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "converted",
		"message": "Conversational conversion complete",
	})
}

func (s *Server) handleSceneRevisions(w http.ResponseWriter, r *http.Request) {
	sceneID := chi.URLParam(r, "id")
	revs, _ := s.database.ListRevisions("scene", sceneID)
	if revs == nil {
		revs = []models.Revision{}
	}
	writeJSON(w, http.StatusOK, revs)
}

func (s *Server) handleShotRevisions(w http.ResponseWriter, r *http.Request) {
	shotID := chi.URLParam(r, "id")
	revs, _ := s.database.ListRevisions("shot", shotID)
	if revs == nil {
		revs = []models.Revision{}
	}
	writeJSON(w, http.StatusOK, revs)
}

func (s *Server) handleRevertRevision(w http.ResponseWriter, r *http.Request) {
	revID := chi.URLParam(r, "id")
	rev, err := s.database.GetRevision(revID)
	if err != nil {
		http.Error(w, "revision not found", http.StatusNotFound)
		return
	}

	if rev.EntityType == "scene" {
		var sc models.Scene
		if err := json.Unmarshal(rev.SnapshotJSON, &sc); err == nil {
			_ = s.database.UpdateScene(&sc)
		}
	} else if rev.EntityType == "shot" {
		var sh models.Shot
		if err := json.Unmarshal(rev.SnapshotJSON, &sh); err == nil {
			_ = s.database.UpdateShot(&sh)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reverted", "revision_id": revID})
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
