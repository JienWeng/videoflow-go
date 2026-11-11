package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"videoflow-go/internal/models"
)

// Include every identity field and all metadata, preserving case and JSON number
// precision. File location is deliberately excluded; file bytes have a separate
// binding. UpdatedAt makes even an explicit identity reapproval a new revision.
func voiceAssetIdentity(asset *models.Asset) (json.RawMessage, error) {
	decode := func(raw json.RawMessage) (any, error) {
		if len(raw) == 0 {
			return nil, nil
		}
		if !json.Valid(raw) {
			return nil, fmt.Errorf("invalid identity JSON")
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
	metadata, err := decode(asset.MetadataJSON)
	if err != nil {
		return nil, fmt.Errorf("asset identity metadata is invalid: %w", err)
	}
	tags, err := decode(asset.TagsJSON)
	if err != nil {
		return nil, fmt.Errorf("asset identity tags are invalid: %w", err)
	}
	return json.Marshal(map[string]any{"id": asset.ID, "project_id": asset.ProjectID, "name": asset.Name, "type": asset.Type, "description": asset.Description, "character_id": asset.CharacterID, "tags": tags, "metadata": metadata, "updated_at": asset.UpdatedAt})
}
