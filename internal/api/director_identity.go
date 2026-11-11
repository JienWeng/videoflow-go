package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"videoflow-go/internal/models"
)

func (s *Server) directorImageInputs(ctx context.Context, project *string, references []string) ([]string, string, error) {
	if !s.cfg.UseLocalText {
		return references, "", nil
	}
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	if len(binding.Selection.CharacterIDs) == 0 && len(binding.Selection.AssetIDs) == 0 {
		return references, "", nil
	}
	if project == nil || *project != binding.ProjectID {
		return nil, "", fmt.Errorf("selected identity context belongs to another project")
	}
	snapshot, _ := ctx.Value(directorSnapshotKey{}).(*directorContext)
	var err error
	if snapshot == nil {
		snapshot, err = s.buildDirectorContext(binding.ProjectID, binding.Selection)
	}
	if err != nil {
		return nil, "", err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ref := range references {
		if !seen[ref] {
			out = append(out, ref)
			seen[ref] = true
		}
	}
	for _, item := range snapshot.Assets {
		assetState, e := s.database.GetAsset(item.ID)
		if e != nil {
			return nil, "", e
		}
		if !models.ReferenceApproved(assetState.MetadataJSON) {
			return nil, "", fmt.Errorf("reference %s requires consistency review in Assets", item.Name)
		}
		if !item.Available {
			return nil, "", fmt.Errorf("approved reference %s is unavailable; repair selection before generation", item.Name)
		}
		asset, err := s.database.GetAsset(item.ID)
		if err != nil {
			return nil, "", err
		}
		ext := strings.ToLower(filepath.Ext(asset.FilePath))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		default:
			continue
		}
		image, err := s.imageURL(asset)
		if err != nil {
			return nil, "", err
		}
		if !seen[image] {
			out = append(out, image)
			seen[image] = true
		}
	}
	if len(out) > 8 {
		return nil, "", fmt.Errorf("selected context exceeds eight image references; explicitly reduce selection before generation")
	}
	raw, _ := json.Marshal(snapshot)
	return out, "\nAuthoritative selected identity constraints and reference bindings (preserve object kind, colors and speaker identity): " + string(raw), nil
}
func (s *Server) handleAssetIdentity(w http.ResponseWriter, r *http.Request) {
	binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
	asset, err := s.database.GetAsset(chi.URLParam(r, "id"))
	if err != nil || asset.ProjectID == nil || *asset.ProjectID != binding.ProjectID {
		http.Error(w, "asset not found in active project", 404)
		return
	}
	var body struct {
		ExpectedUpdatedAt time.Time         `json:"expected_updated_at"`
		LockedTraits      map[string]string `json:"locked_traits"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&body) != nil || len(body.LockedTraits) > 32 {
		http.Error(w, "invalid identity traits", 400)
		return
	}
	if body.ExpectedUpdatedAt.IsZero() || !body.ExpectedUpdatedAt.Equal(asset.UpdatedAt) {
		http.Error(w, "asset changed; reopen it before changing identity locks", 409)
		return
	}
	for key, value := range body.LockedTraits {
		if strings.TrimSpace(key) == "" || len(key) > 64 || strings.TrimSpace(value) == "" || len(value) > 500 {
			http.Error(w, "each trait needs a name (up to 64 bytes) and value (up to 500 bytes)", 400)
			return
		}
	}
	meta := map[string]any{}
	if len(asset.MetadataJSON) > 0 && json.Unmarshal(asset.MetadataJSON, &meta) != nil {
		http.Error(w, "asset metadata needs repair", 409)
		return
	}
	if meta == nil {
		meta = map[string]any{}
	}
	meta["locked_traits"] = body.LockedTraits
	asset.MetadataJSON, _ = json.Marshal(meta)
	if err = s.database.UpdateAssetReview(binding.ProjectID, asset.ID, body.ExpectedUpdatedAt, asset.MetadataJSON); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	saved, _ := s.database.GetAsset(asset.ID)
	writeJSON(w, 200, saved)
}
