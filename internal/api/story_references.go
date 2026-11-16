package api

import (
	"context"
	"encoding/json"
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
	Status              string                `json:"status"`
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
	} else if s.cfg.UseLocalText {
		ctx = guidedStoryContext(ctx, *c.ProjectID, []string{c.ID})
		binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
		binding.Selection.referenceCandidate = c.ID
		ctx = context.WithValue(ctx, directorBindingKey{}, binding)
	} else {
		for _, id := range existing {
			asset, e := s.database.GetAsset(id)
			if e != nil || asset.ProjectID == nil || *asset.ProjectID != *c.ProjectID {
				return nil, fmt.Errorf("character reference is not in this project")
			}
			if !models.ReferenceApproved(asset.MetadataJSON) {
				continue
			}
			image, e := s.imageURL(asset)
			if e == nil {
				refs = append(refs, image)
			}
		}
	}
	prompt := fmt.Sprintf("Character reference sheet showing front, side and full body views of %s. Appearance: %s. Description: %s. Visual rules: %s", c.Name, c.Appearance, c.Description, c.VisualRulesJSON)
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

func (s *Server) generateStoryboards(ctx context.Context, scenes []models.Scene) error {
	for i := range scenes {
		scene := &scenes[i]
		var cast []string
		if err := json.Unmarshal(scene.CharacterIDsJSON, &cast); err != nil {
			return err
		}
		brief, err := s.storySceneBrief(*scene.ProjectID, sceneBrief(scene), cast, nil)
		if err != nil {
			return err
		}
		var snapshot map[string]any
		if err = json.Unmarshal(scene.SceneJSON, &snapshot); err != nil {
			return err
		}
		if snapshot == nil {
			snapshot = map[string]any{}
		}
		snapshot["generation_brief"] = brief
		scene.SceneJSON, err = json.Marshal(snapshot)
		if err != nil {
			return err
		}
		var refs []string
		if !s.cfg.UseLocalText {
			refs, err = s.sceneReferences(scene)
			if err != nil {
				return err
			}
		}
		// Local generation gets images exclusively from the current validated
		// director snapshot, never detached references from an older scene brief.
		sceneCtx := brief.context(guidedStoryContext(ctx, *scene.ProjectID, brief.CharacterIDs))
		images, err := s.generateAssets(sceneCtx, scene.ProjectID, nil, scene.ID, scene.Title+" keyframe", "storyboard", scene.Title+". "+scene.Summary+"\n"+brief.prompt(), scene.AspectRatio, refs)
		if err != nil {
			return err
		}
		var ids []string
		json.Unmarshal(scene.AssetIDsJSON, &ids)
		for _, image := range images {
			ids = append(ids, image.ID)
		}
		scene.AssetIDsJSON, _ = json.Marshal(ids)
		if err = s.database.UpdateScene(scene); err != nil {
			return err
		}
	}
	return nil
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
	if json.Unmarshal(op.ResultJSON, &state) != nil || op.Status != "succeeded" || state.Status != "awaiting_character_review" || !s.cfg.UseLocalText || state.ProjectID != project {
		http.Error(w, "this story is not waiting for character review", 409)
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
				http.Error(w, "approve each character reference before continuing", 409)
				return
			}
			if _, e = s.imageURL(asset); e != nil {
				http.Error(w, "character reference is unavailable", 409)
				return
			}
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
		workCtx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancel()
		workCtx = context.WithValue(workCtx, directorBindingKey{}, binding)
		workCtx = state.Brief.context(guidedStoryContext(workCtx, project, state.Cast))
		workCtx = context.WithValue(workCtx, mediaRoutesKey{}, state.Routes)
		if e := s.generateStoryboards(workCtx, scenes); e != nil {
			state.Status = "failed"
			raw, _ := json.Marshal(state)
			message := e.Error()
			s.database.UpdateOp(op.ID, "failed", raw, &message)
			return
		}
		state.Stages = append(state.Stages, "visuals")
		state.Status = "awaiting_reference_review"
		raw, _ := json.Marshal(state)
		s.database.UpdateOp(op.ID, "succeeded", raw, nil)
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": "running", "character_references": true})
}
