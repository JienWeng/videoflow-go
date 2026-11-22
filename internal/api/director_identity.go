package api

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"videoflow-go/internal/models"
)

func (s *Server) directorImageInputs(ctx context.Context, project *string, references []string) ([]string, string, error) {
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
			return nil, "", fmt.Errorf("reference %s is excluded from generation; replace it in Library", item.Name)
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
