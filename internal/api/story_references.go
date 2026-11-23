package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"videoflow-go/internal/models"
)

type storyContinuation struct {
	ProjectID           string                `json:"project_id"`
	ScriptID            string                `json:"script_id"`
	SceneIDs            []string              `json:"scene_ids"`
	Stages              []string              `json:"stages"`
	Brief               generationBrief       `json:"brief"`
	Routes              map[string]mediaRoute `json:"media_routes"`
	Created             []string              `json:"created_character_ids"`
	Cast                []string              `json:"cast_character_ids"`
	References          []string              `json:"character_reference_ids"`
	CharacterReferences bool                  `json:"character_references"`
	AssetDecisions      []plannedAsset        `json:"story_assets,omitempty"`
	AutomaticAssets     bool                  `json:"automatic_assets,omitempty"`
	AssetReferences     []string              `json:"story_asset_ids,omitempty"`
	Status              string                `json:"status"`
	// StoryboardsMade is how many storyboard pictures the visuals stage made; zero when
	// every scene's clips use its connected pictures as references instead.
	StoryboardsMade *int `json:"storyboards_made,omitempty"`
}

func (s *Server) generateCharacterSheet(ctx context.Context, c *models.Character, onlyMissing bool, style string) (*models.Character, error) {
	if c.ProjectID == nil {
		return nil, fmt.Errorf("character has no project")
	}
	value, _ := s.referenceLocks.LoadOrStore(c.ID, make(chan struct{}, 1))
	lock := value.(chan struct{})
	if onlyMissing {
		select {
		case lock <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		select {
		case lock <- struct{}{}:
		default:
			return nil, fmt.Errorf("reference generation is already running for this character; check Activity")
		}
	}
	defer func() { <-lock }()
	current, err := s.database.GetCharacter(c.ID)
	if err != nil {
		return nil, err
	}
	if current.ProjectID == nil || *current.ProjectID != *c.ProjectID {
		return nil, fmt.Errorf("character project changed")
	}
	c = current
	var existing []string
	if json.Unmarshal(c.ReferenceAssetIDsJSON, &existing) != nil {
		return nil, fmt.Errorf("character has invalid reference bindings")
	}
	if onlyMissing && len(existing) > 0 {
		return c, nil
	}
	var refs []string
	if onlyMissing {
		cast, _ := json.Marshal([]string{c.ID})
		refs, err = s.sceneReferences(&models.Scene{ProjectID: c.ProjectID, CharacterIDsJSON: cast})
		if err != nil {
			return nil, err
		}
	} else {
		ctx = guidedStoryContext(ctx, *c.ProjectID, []string{c.ID})
		binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
		binding.Selection.referenceCandidate = c.ID
		ctx = context.WithValue(ctx, directorBindingKey{}, binding)
	}
	// The Seedance guides ask for one face close-up and one full-body view of a character,
	// not a board of many angles, which the video model can read as several people.
	prompt := fmt.Sprintf("Character reference sheet of %s with exactly two views of the same character side by side on a plain, light, uncluttered background. Left: a large, sharp head-and-shoulders close-up, facing the camera, neutral expression, even lighting. Right: one full-body front view in a neutral standing pose, the entire character visible from head to feet/paws, including ears and tail, with margins and no cropped limbs. Face, proportions, clothing, colors and identifying features are identical in both views. No side or back views, no other poses, no story action, no extra characters, no text or labels. Adapt to the actual species and anatomy; do not invent limbs. Appearance: %s. Description: %s. Visual rules: %s", c.Name, c.Appearance, c.Description, c.VisualRulesJSON)
	if style != "" {
		prompt += "\nVisual style: " + style
	}
	images, err := s.generateAssets(ctx, c.ProjectID, &c.ID, "", c.Name+" reference", "character_reference", prompt, "1:1", refs)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("reference provider returned no image for %s", c.Name)
	}
	ids := []string{}
	for _, image := range images {
		ids = append(ids, image.ID)
	}
	if onlyMissing {
		err = s.database.AppendCharacterReferences(c.ID, *c.ProjectID, ids)
	} else {
		err = s.database.ReplaceUnapprovedCharacterReferences(c.ID, *c.ProjectID, ids)
	}
	if err != nil {
		return nil, err
	}
	return s.database.GetCharacter(c.ID)
}

func (s *Server) storyCharacterReferences(ctx context.Context, project string, cast []string, style string) ([]string, bool, error) {
	ids := []string{}
	approved := true
	seen := map[string]bool{}
	chars := []models.Character{}
	for _, id := range cast {
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			return ids, false, fmt.Errorf("story character is not in this project")
		}
		chars = append(chars, *c)
	}
	// Missing sheets are made side by side; the loop below then finds each one made.
	if batch := s.makeCharacterSheets(ctx, chars, true, style, nil); len(batch.Failed) > 0 {
		return ids, false, errors.New(batch.Failed[0].Error)
	}
	for _, id := range cast {
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			return ids, false, fmt.Errorf("story character is not in this project")
		}
		c, err = s.generateCharacterSheet(guidedStoryContext(ctx, project, []string{id}), c, true, style)
		if err != nil {
			return ids, false, err
		}
		var refs []string
		if json.Unmarshal(c.ReferenceAssetIDsJSON, &refs) != nil || len(refs) == 0 {
			return ids, false, fmt.Errorf("character references are missing")
		}
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			asset, err := s.database.GetAsset(ref)
			if err != nil || asset.ProjectID == nil || *asset.ProjectID != project {
				return ids, false, fmt.Errorf("character reference is not in this project")
			}
			if _, err = s.imageURL(asset); err != nil {
				return ids, false, err
			}
			ids = append(ids, ref)
			approved = approved && models.ReferenceApproved(asset.MetadataJSON)
		}
	}
	return ids, approved, nil
}

// storyboardStory makes the storyboard of each scene whose clips start from one, with the
// generator a scene render uses, and reports how many it made. A scene whose clips are
// sent its connected pictures as references gets none: the clip would never use it, and
// a render makes one later if the scene is switched to first frame.
// The scene's brief is first brought up to date with its cast's notes and reference
// pictures, which exist only now; it is saved with or without a new storyboard.
func (s *Server) storyboardStory(ctx context.Context, scenes []models.Scene) (int, error) {
	made := 0
	for i := range scenes {
		scene := &scenes[i]
		var cast []string
		if err := json.Unmarshal(scene.CharacterIDsJSON, &cast); err != nil {
			return made, err
		}
		brief, err := s.storySceneBrief(*scene.ProjectID, sceneBrief(scene), cast, nil)
		if err != nil {
			return made, err
		}
		brief.Style = s.sceneVisualStyle(scene)
		var snapshot map[string]any
		if err = json.Unmarshal(scene.SceneJSON, &snapshot); err != nil {
			return made, err
		}
		if snapshot == nil {
			snapshot = map[string]any{}
		}
		snapshot["generation_brief"] = brief
		scene.SceneJSON, err = json.Marshal(snapshot)
		if err != nil {
			return made, err
		}
		sceneCtx := guidedStoryContext(ctx, *scene.ProjectID, nil)
		if s.sceneUsesReferences(sceneCtx, scene) {
			if err = s.database.UpdateScene(scene); err != nil {
				return made, err
			}
			continue
		}
		if _, err = s.generateMatchingStoryboard(sceneCtx, scene); err != nil {
			return made, err
		}
		made++
	}
	return made, nil
}

func (s *Server) handleContinueStory(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	op, err := s.database.GetOp(chi.URLParam(r, "id"))
	if err != nil || op.ProjectID == nil || *op.ProjectID != project || op.Kind != "video_generation" {
		http.Error(w, "story operation not found in this project", 404)
		return
	}
	var state storyContinuation
	if json.Unmarshal(op.ResultJSON, &state) != nil || op.Status != "succeeded" || state.Status != "awaiting_character_review" || state.ProjectID != project {
		http.Error(w, "this story is not paused before storyboarding", 409)
		return
	}
	scenes := []models.Scene{}
	cast := map[string]bool{}
	for _, id := range state.Cast {
		cast[id] = true
	}
	actual := map[string]bool{}
	if len(state.SceneIDs) == 0 || state.ScriptID == "" {
		http.Error(w, "saved story is incomplete", 409)
		return
	}
	for _, id := range state.SceneIDs {
		scene, e := s.database.GetScene(id)
		if e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
			http.Error(w, "saved scene is not in this project", 409)
			return
		}
		var ids []string
		if json.Unmarshal(scene.CharacterIDsJSON, &ids) != nil {
			http.Error(w, "invalid scene cast", 409)
			return
		}
		for _, id := range ids {
			actual[id] = true
			if !cast[id] {
				http.Error(w, "scene cast changed; review the saved story", 409)
				return
			}
		}
		scenes = append(scenes, *scene)
	}
	if len(actual) != len(cast) {
		http.Error(w, "scene cast changed; review the saved story", 409)
		return
	}
	ctx := guidedStoryContext(r.Context(), project, state.Cast)
	ctx = context.WithValue(ctx, mediaRoutesKey{}, state.Routes)
	// Validate the exact current bound references without generating anything.
	for _, id := range state.Cast {
		c, e := s.database.GetCharacter(id)
		if e != nil || c.ProjectID == nil || *c.ProjectID != project {
			http.Error(w, "story character is unavailable", 409)
			return
		}
		var refs []string
		if json.Unmarshal(c.ReferenceAssetIDsJSON, &refs) != nil || len(refs) == 0 {
			http.Error(w, "character references are missing", 409)
			return
		}
		for _, id := range refs {
			asset, e := s.database.GetAsset(id)
			if e != nil || asset.ProjectID == nil || *asset.ProjectID != project || !models.ReferenceApproved(asset.MetadataJSON) {
				http.Error(w, "replace missing or excluded character references before continuing", 409)
				return
			}
			if _, e = s.imageURL(asset); e != nil {
				http.Error(w, "character reference is unavailable", 409)
				return
			}
		}
	}
	for _, id := range state.AssetReferences {
		asset, e := s.database.GetAsset(id)
		if e != nil || asset.ProjectID == nil || *asset.ProjectID != project || !models.ReferenceApproved(asset.MetadataJSON) {
			http.Error(w, "replace missing or excluded props and locations before continuing", 409)
			return
		}
		if _, e = s.imageURL(asset); e != nil {
			http.Error(w, "story asset reference is unavailable", 409)
			return
		}
	}
	for i := range scenes {
		bound, _, e := s.directorGenerationContext(ctx, &project, scenes[i].ID, nil)
		if e == nil {
			_, _, e = s.directorImageInputs(bound, &project, nil)
		}
		if e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
	}
	for _, kind := range []string{"image", "reference", "video"} {
		route, ok := state.Routes[kind]
		if !ok || route.Provider == "" || route.Model == "" {
			http.Error(w, "saved media routes are incomplete", 409)
			return
		}
	}
	if missing := s.generationPrerequisites(ctx, state.Brief); len(missing) > 0 {
		http.Error(w, missing[0]["reason"], 400)
		return
	}
	select {
	case s.operationSlots <- struct{}{}:
	default:
		http.Error(w, "Three background operations are already running. Check Activity.", 429)
		return
	}
	claimed, err := s.database.ClaimStoryContinuation(op.ID, project, op.ResultJSON)
	if err != nil || !claimed {
		<-s.operationSlots
		http.Error(w, "this story has already continued or changed; refresh its status", 409)
		return
	}
	state.Stages = append(state.Stages, "character_review")
	state.Status = "storyboarding"
	raw, _ := json.Marshal(state)
	if err = s.database.UpdateOp(op.ID, "running", raw, nil); err != nil {
		<-s.operationSlots
		http.Error(w, "could not save continuation state; check Activity", 500)
		return
	}
	binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		defer func() { <-s.operationSlots }()
		workCtx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		workCtx = context.WithValue(workCtx, directorBindingKey{}, binding)
		workCtx = state.Brief.context(guidedStoryContext(workCtx, project, state.Cast))
		workCtx = context.WithValue(workCtx, mediaRoutesKey{}, state.Routes)
		made, e := s.storyboardStory(workCtx, scenes)
		if e != nil {
			state.Status = models.StatusFailed
			raw, _ := json.Marshal(state)
			message := e.Error()
			s.database.UpdateOp(op.ID, models.StatusFailed, raw, &message)
			return
		}
		state.Stages = append(state.Stages, "visuals")
		state.StoryboardsMade = &made
		state.Status = "ready_to_render"
		raw, _ := json.Marshal(state)
		s.database.UpdateOp(op.ID, "succeeded", raw, nil)
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": models.StatusRunning, "character_references": true})
}

func (s *Server) handleExcludeStoryAsset(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	op, err := s.database.GetOp(chi.URLParam(r, "id"))
	if err != nil || op.ProjectID == nil || *op.ProjectID != project || op.Kind != "video_generation" {
		http.Error(w, "story not found", 404)
		return
	}
	var state storyContinuation
	if json.Unmarshal(op.ResultJSON, &state) != nil || op.Status != "succeeded" || state.Status != "awaiting_character_review" || state.ProjectID != project {
		http.Error(w, "story is not paused before storyboarding", 409)
		return
	}
	var body struct {
		AssetID string `json:"asset_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.AssetID == "" {
		http.Error(w, "asset_id is required", 400)
		return
	}
	found := false
	ids := []string{}
	for _, id := range state.AssetReferences {
		if id == body.AssetID {
			found = true
		} else {
			ids = append(ids, id)
		}
	}
	if !found {
		http.Error(w, "asset is not selected for this story", 404)
		return
	}
	state.AssetReferences = ids
	decisions := []plannedAsset{}
	for _, item := range state.AssetDecisions {
		if item.AssetID != body.AssetID {
			decisions = append(decisions, item)
		}
	}
	state.AssetDecisions = decisions
	next, _ := json.Marshal(state)
	if err = s.database.ExcludeStoryAsset(op.ID, project, body.AssetID, state.SceneIDs, op.ResultJSON, next); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, state)
}
