package api

import (
	"context"
	"encoding/json"
	"strings"

	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
	"videoflow-go/internal/seedance"
)

// How a scene's clips are given pictures. In reference mode the pictures connected to the
// scene (its cast, places and props) go to the video model as references, the way
// Seedance's own canvas connects them; in first-frame mode a storyboard picture of the
// opening is made first and the clip starts from it.
const (
	inputReferences = "references"
	inputFirstFrame = "first_frame"
)

// The guides advise against using a model's whole reference allowance: 4-5 pictures,
// characters first, keep the model sure of what matters.
const (
	maxReferencePictures   = 6
	maxReferenceCharacters = 3
	maxReferencePlaces     = 2
	maxReferenceProps      = 1
)

// referencePicture is one picture a clip is sent, in the order the prompt numbers them.
type referencePicture struct {
	AssetID string `json:"asset_id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // character, place or prop
	Sheet   bool   `json:"sheet,omitempty"`
	// CharacterID is whose picture it is, for a character.
	CharacterID string `json:"character_id,omitempty"`
}

// referencePictures are the pictures a clip of the scene is sent. With a unit, only the
// characters named in that clip's shots or speaking in it are included, so nobody absent
// from the clip is put into it. limit is the model's own maximum.
func (s *Server) referencePictures(scene *models.Scene, unit *renderUnit, limit int) []referencePicture {
	if limit > maxReferencePictures {
		limit = maxReferencePictures
	}
	usable := func(id string) *models.Asset {
		asset, err := s.database.GetAsset(id)
		if err != nil || asset.ProjectID == nil || *asset.ProjectID != *scene.ProjectID || !imageAsset(asset.FilePath) || !models.ReferenceApproved(asset.MetadataJSON) {
			return nil
		}
		return asset
	}
	inClip := func(string) bool { return true }
	if unit != nil {
		said := ""
		mine := map[string]bool{}
		for _, id := range unit.ShotIDs {
			mine[id] = true
			if shot, err := s.database.GetShot(id); err == nil {
				said += " " + strings.ToLower(shot.Prompt)
			}
		}
		var script struct {
			Lines []SpokenLine `json:"spoken_lines"`
		}
		_ = json.Unmarshal(scene.SceneJSON, &script)
		speakers := map[string]bool{}
		for _, line := range script.Lines {
			if mine[sceneProduction(scene).DialogueShots[line.ID]] {
				speakers[line.CharacterID] = true
			}
		}
		inClip = func(key string) bool {
			id, name, _ := strings.Cut(key, "\x00")
			return speakers[id] || strings.Contains(said, strings.ToLower(name))
		}
	}

	out := []referencePicture{}
	cast := []string{}
	_ = json.Unmarshal(scene.CharacterIDsJSON, &cast)
	cast = append(cast, sceneBrief(scene).CharacterIDs...)
	seen := map[string]bool{}
	characters := 0
	for _, id := range cast {
		character, err := s.database.GetCharacter(id)
		if err != nil || character == nil || seen[id] || characters == maxReferenceCharacters || !inClip(id+"\x00"+character.Name) {
			continue
		}
		seen[id] = true
		refs := []string{}
		_ = json.Unmarshal(character.ReferenceAssetIDsJSON, &refs)
		for _, ref := range refs {
			if asset := usable(ref); asset != nil {
				out = append(out, referencePicture{AssetID: asset.ID, Name: character.Name, Kind: "character", Sheet: asset.Type == "character_reference", CharacterID: id})
				characters++
				break
			}
		}
	}
	places, props := 0, 0
	for _, id := range s.sceneBoundAssets(scene) {
		asset := usable(id)
		if asset == nil || seen[id] {
			continue
		}
		seen[id] = true
		switch {
		case asset.Type == "location" && places < maxReferencePlaces:
			out = append(out, referencePicture{AssetID: asset.ID, Name: asset.Name, Kind: "place"})
			places++
		case asset.Type == "prop" && props < maxReferenceProps:
			out = append(out, referencePicture{AssetID: asset.ID, Name: asset.Name, Kind: "prop"})
			props++
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// referenceModePictures are the pictures a render of the scene would send as references:
// none when the owner chose first frame, the video model takes no reference pictures, or
// nothing usable is connected. None means first-frame mode, which needs a storyboard.
func (s *Server) referenceModePictures(scene *models.Scene, settings productionSettings, model *providers.Model) []referencePicture {
	if settings.VideoInput == inputFirstFrame || model == nil || model.ReferenceImages <= 0 {
		return nil
	}
	return s.referencePictures(scene, nil, model.ReferenceImages)
}

// sceneUsesReferences reports whether a render of the scene right now would send its
// connected pictures as references. Such a scene's clips never use a storyboard picture,
// so none is made for it and nothing waits on one. It is the render plan's own answer;
// while no plan can be made yet (shots still to fix, say) the plan's picture rule alone
// decides, so story creation and the later render agree.
func (s *Server) sceneUsesReferences(ctx context.Context, scene *models.Scene) bool {
	if plan, err := s.sceneProductionPlan(ctx, scene); err == nil {
		return plan.VideoInput == inputReferences
	}
	_, model, err := s.productionVideoModel(ctx)
	if err != nil {
		return false
	}
	return len(s.referenceModePictures(scene, sceneProduction(scene), model)) > 0
}

// clipReferences reads the pictures a frozen clip request is sent with.
func clipReferences(payload map[string]any) []seedance.Reference {
	raw, _ := json.Marshal(payload["reference_pictures"])
	var pictures []referencePicture
	_ = json.Unmarshal(raw, &pictures)
	out := []seedance.Reference{}
	for _, p := range pictures {
		out = append(out, seedance.Reference{Kind: p.Kind, Name: p.Name, Sheet: p.Sheet})
	}
	return out
}

// attachReferenceVoices adds the voice samples of the characters who speak in a
// reference-mode clip: the files in audio_urls, in order, and whose each one is in
// reference_voices, which the prompt writer binds to @Audio 1, @Audio 2, ... Only a
// clip with sound, sent its speakers' pictures, through a service that takes voice
// recordings, gets any.
func (s *Server) attachReferenceVoices(ctx context.Context, job *models.RenderJob) {
	var payload map[string]any
	if json.Unmarshal(job.RequestJSON, &payload) != nil || job.SceneID == nil || job.ProjectID == nil {
		return
	}
	delete(payload, "audio_urls")
	delete(payload, "reference_voices")
	defer func() { job.RequestJSON = voiceJSON(payload) }()
	raw, _ := json.Marshal(payload["reference_pictures"])
	var pictures []referencePicture
	_ = json.Unmarshal(raw, &pictures)
	policy, _ := payload["voice_policy"].(map[string]any)
	scene, err := s.database.GetScene(*job.SceneID)
	if err != nil || len(pictures) == 0 || policy["mode"] != "native" || sceneBrief(scene).ConversationMode == "narration" {
		return
	}
	client, _, err := s.routes().Media(job.Provider, job.Model, job.ProviderEndpoint)
	if err != nil {
		return
	}
	model, err := client.Model(ctx, job.Model, "video")
	if err != nil || model.ReferenceAudios == 0 {
		return
	}
	// Who speaks in this clip: a line pinned to a shot is spoken only in that shot's clip.
	unit, settings := renderRequestUnit(job.RequestJSON), sceneProduction(scene)
	mine := map[string]bool{}
	for _, id := range unit.ShotIDs {
		mine[id] = true
	}
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script)
	speaks := map[string]bool{}
	for _, line := range script.Lines {
		target := settings.DialogueShots[line.ID]
		if strings.TrimSpace(line.Text) != "" && (target == "" || len(mine) == 0 || mine[target]) {
			speaks[line.CharacterID] = true
		}
	}
	files, names := []string{}, []string{}
	for _, p := range pictures {
		if p.CharacterID == "" || !speaks[p.CharacterID] || len(files) == model.ReferenceAudios {
			continue
		}
		if sample := s.voiceSample(*job.ProjectID, p.CharacterID); sample != nil {
			files, names = append(files, sample.FilePath), append(names, p.Name)
		}
	}
	if len(files) > 0 {
		payload["audio_urls"], payload["reference_voices"] = files, names
	}
}

// attachReferencePictures puts a reference-mode clip's pictures into its request: the
// files in image_urls, in order, and what each one is in reference_pictures, which the
// prompt writer binds to @Image 1, @Image 2, ... Nothing is attached for other clips.
func (s *Server) attachReferencePictures(job *models.RenderJob, payload map[string]any, limit int) error {
	if payload["video_input"] != inputReferences || job.SceneID == nil {
		return nil
	}
	scene, err := s.database.GetScene(*job.SceneID)
	if err != nil {
		return err
	}
	unit := renderRequestUnit(job.RequestJSON)
	var pictures []referencePicture
	if len(unit.ShotIDs) > 0 {
		pictures = s.referencePictures(scene, &unit, limit)
	} else {
		pictures = s.referencePictures(scene, nil, limit)
	}
	if len(pictures) == 0 {
		// Nothing connected shows in this clip: it is made from the words alone.
		delete(payload, "image_urls")
		delete(payload, "reference_pictures")
		return nil
	}
	files := []string{}
	for _, p := range pictures {
		asset, err := s.database.GetAsset(p.AssetID)
		if err != nil {
			return err
		}
		files = append(files, asset.FilePath) // turned into the picture itself just before sending
	}
	payload["image_urls"], payload["reference_pictures"] = files, pictures
	return nil
}
