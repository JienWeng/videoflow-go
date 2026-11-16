package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleUpdateAssetDetails(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	asset, e := s.database.GetAsset(chi.URLParam(r, "id"))
	if err != nil || e != nil || asset.ProjectID == nil || *asset.ProjectID != project {
		http.Error(w, "asset not found", 404)
		return
	}
	var body struct {
		Expected    time.Time         `json:"expected_updated_at"`
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
		Locks       map[string]string `json:"locked_traits"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || body.Expected.IsZero() {
		http.Error(w, "asset details and expected_updated_at are required", 400)
		return
	}
	if !body.Expected.Equal(asset.UpdatedAt) {
		http.Error(w, "asset changed; reopen it before saving", 409)
		return
	}
	if body.Name != nil {
		if strings.TrimSpace(*body.Name) == "" || len(*body.Name) > 200 {
			http.Error(w, "asset name must contain 1 to 200 bytes", 400)
			return
		}
		asset.Name = strings.TrimSpace(*body.Name)
	}
	var meta map[string]any
	if json.Unmarshal(asset.MetadataJSON, &meta) != nil {
		http.Error(w, "asset metadata needs repair", 409)
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	changed := false
	if body.Description != nil {
		if len(*body.Description) > 8000 {
			http.Error(w, "description is too long", 400)
			return
		}
		changed = *body.Description != asset.Description
		asset.Description = *body.Description
	}
	if body.Locks != nil {
		if len(body.Locks) > 32 {
			http.Error(w, "too many identity locks", 400)
			return
		}
		old, _ := meta["locked_traits"].(map[string]any)
		if len(old) != len(body.Locks) {
			changed = true
		}
		for key, value := range body.Locks {
			if strings.TrimSpace(key) == "" || len(key) > 64 || strings.TrimSpace(value) == "" || len(value) > 500 {
				http.Error(w, "each identity trait requires a valid name and value", 400)
				return
			}
			if old[key] != value {
				changed = true
			}
		}
		meta["locked_traits"] = body.Locks
	}
	if changed {
		meta["consistency_review"] = "pending"
		delete(meta, "consistency_reviewed_at")
	}
	asset.MetadataJSON, _ = json.Marshal(meta)
	if e = s.database.UpdateAssetDetails(asset, body.Expected); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	asset, _ = s.database.GetAsset(asset.ID)
	s.broker.Publish(map[string]any{"type": "asset_changed", "asset_id": asset.ID})
	writeJSON(w, 200, asset)
}
