package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (s *Server) handleAssetReview(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	asset, e := s.database.GetAsset(chi.URLParam(r, "id"))
	if e != nil || asset.ProjectID == nil || *asset.ProjectID != project {
		http.Error(w, "asset not found in active project", 404)
		return
	}
	var body struct {
		Approved *bool     `json:"approved"`
		Expected time.Time `json:"expected_updated_at"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&body) != nil || body.Approved == nil {
		http.Error(w, "choose approve or reject", 400)
		return
	}
	if body.Expected.IsZero() || !body.Expected.Equal(asset.UpdatedAt) {
		http.Error(w, "asset changed; reopen it before reviewing", 409)
		return
	}
	meta := map[string]any{}
	if json.Unmarshal(asset.MetadataJSON, &meta) != nil || meta == nil {
		http.Error(w, "asset metadata needs repair before review", 409)
		return
	}
	status := "rejected"
	if *body.Approved {
		status = "approved"
	}
	if *body.Approved && asset.Type == "storyboard" {
		if sceneID, ok := meta["scene_id"].(string); ok {
			scene, e := s.database.GetScene(sceneID)
			if e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
				http.Error(w, "storyboard scene is unavailable", 409)
				return
			}
			revision, e := s.sceneInputRevision(scene)
			if e != nil {
				http.Error(w, e.Error(), 409)
				return
			}
			meta["scene_input_revision"] = revision
		}
	}
	meta["consistency_review"] = status
	meta["consistency_reviewed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, _ := json.Marshal(meta)
	if e = s.database.UpdateAssetReview(project, asset.ID, body.Expected, data); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	saved, e := s.database.GetAsset(asset.ID)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	s.broker.Publish(map[string]any{"type": "asset_changed", "asset_id": asset.ID})
	writeJSON(w, 200, saved)
}
