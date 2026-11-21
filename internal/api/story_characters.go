package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

var storyCharacterKey = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Guided creation has its own explicit cast, even when empty. Keep the selected
// native thread, but never inherit or rewrite its saved reference selection.
func guidedStoryContext(ctx context.Context, project string, ids []string) context.Context {
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	binding.ProjectID = project
	binding.Selection = directorSelection{CharacterIDs: append([]string{}, ids...)}
	ctx = context.WithValue(ctx, directorBindingKey{}, binding)
	return context.WithValue(ctx, directorSnapshotKey{}, (*directorContext)(nil))
}

// Resolve the model's temporary keys before any writes. Names never authorize
// adopting an unselected identity, and model-supplied database IDs are not trusted.
func (s *Server) prepareStoryCharacters(project string, b generationBrief, draft *agents.ScriptDraft) ([]models.Character, map[string]string, error) {
	if draft == nil {
		return nil, nil, fmt.Errorf("story draft is missing")
	}
	if len(draft.Characters) > 0 && !b.CreateCharacters {
		return nil, nil, fmt.Errorf("character creation was not enabled; review the story settings")
	}
	if len(draft.Characters) > 8 {
		return nil, nil, fmt.Errorf("story proposes too many characters; choose at most 8")
	}
	allowed := map[string]bool{}
	selected := map[string]bool{}
	ordered := []string{}
	for _, id := range b.CharacterIDs {
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			return nil, nil, fmt.Errorf("selected character is not in this project")
		}
		if !allowed[id] {
			allowed[id] = true
			selected[id] = true
			ordered = append(ordered, id)
		}
	}
	catalog, err := s.database.ListCharacters(project)
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	profiles := map[string]models.Character{}
	for _, c := range catalog {
		names[strings.ToLower(strings.TrimSpace(c.Name))] = c.ID
		profiles[c.ID] = c
	}
	bindings := map[string]string{}
	characters := []models.Character{}
	for i, planned := range draft.Characters {
		name := strings.TrimSpace(planned.Name)
		if !storyCharacterKey.MatchString(planned.Key) || name == "" || len(name) > 120 || strings.TrimSpace(planned.Appearance) == "" || len(planned.Appearance) > 4000 || len(planned.Description) > 2000 || len(planned.Personality) > 2000 {
			return nil, nil, fmt.Errorf("story contains an incomplete character profile or invalid character key")
		}
		key := "new:" + planned.Key
		if _, exists := bindings[key]; exists {
			return nil, nil, fmt.Errorf("story contains a duplicate character key")
		}
		folded := strings.ToLower(name)
		if existing, exists := names[folded]; exists {
			if !selected[existing] {
				return nil, nil, fmt.Errorf("a character named %q already exists; select that character instead of creating a duplicate", name)
			}
			bindings[key] = existing // Selected existing identities remain unchanged.
			c := profiles[existing]
			draft.Characters[i] = agents.CharacterDraft{Key: planned.Key, Name: c.Name, Description: c.Description, Appearance: c.Appearance, Personality: c.Personality}
			continue
		}
		id := "char_" + uuid.New().String()[:8]
		characters = append(characters, models.Character{ID: id, ProjectID: &project, Name: name, Description: planned.Description, Appearance: planned.Appearance, Personality: planned.Personality, VisualRulesJSON: []byte("[]"), VoiceRulesJSON: []byte("[]"), ReferenceAssetIDsJSON: []byte("[]")})
		bindings[key] = id
		allowed[id] = true
		ordered = append(ordered, id)
		names[folded] = id
	}
	if len(allowed) > 8 {
		return nil, nil, fmt.Errorf("story cast exceeds 8 characters; select fewer characters")
	}
	resolve := func(id string) (string, error) {
		if actual, exists := bindings[id]; exists {
			return actual, nil
		}
		if allowed[id] {
			return id, nil
		}
		return "", fmt.Errorf("story uses an unknown character; review its cast before generating images")
	}
	used := map[string]bool{}
	for i := range draft.Scenes {
		scene := &draft.Scenes[i]
		ids := scene.CharacterIDs
		if ids == nil {
			ids = ordered
		}
		if len(ids) > 8 {
			return nil, nil, fmt.Errorf("scene cast exceeds 8 characters")
		}
		cast := []string{}
		seen := map[string]bool{}
		for _, id := range ids {
			actual, err := resolve(id)
			if err != nil {
				return nil, nil, err
			}
			if !seen[actual] {
				cast = append(cast, actual)
				seen[actual] = true
				used[actual] = true
			}
		}
		scene.CharacterIDs = cast
		for j := range scene.SpokenLines {
			line := &scene.SpokenLines[j]
			if line.CharacterID == "" {
				continue
			}
			actual, err := resolve(line.CharacterID)
			if err != nil {
				return nil, nil, err
			}
			if !seen[actual] {
				return nil, nil, fmt.Errorf("story dialogue names a speaker outside its scene cast")
			}
			line.CharacterID = actual
		}
	}
	for _, c := range characters {
		if !used[c.ID] {
			return nil, nil, fmt.Errorf("story proposes an unused character; review the cast")
		}
	}
	return characters, bindings, nil
}

// Build each saved scene's brief from its actual cast, including uncommitted
// newly planned profiles. Never carry other scenes' identity notes/references.
func (s *Server) storySceneBrief(project string, b generationBrief, ids []string, created []models.Character) (generationBrief, error) {
	b.CharacterIDs = append([]string{}, ids...)
	b.CharacterNotes = ""
	b.ReferenceAssetIDs = []string{}
	profiles := map[string]models.Character{}
	for _, c := range created {
		profiles[c.ID] = c
	}
	seen := map[string]bool{}
	for _, id := range ids {
		c, ok := profiles[id]
		if !ok {
			stored, err := s.database.GetCharacter(id)
			if err != nil {
				return b, err
			}
			c = *stored
		}
		if c.ProjectID == nil || *c.ProjectID != project {
			return b, fmt.Errorf("scene character belongs to another project")
		}
		b.CharacterNotes += characterNote(&c)
		var refs []string
		if len(c.ReferenceAssetIDsJSON) > 0 {
			if err := json.Unmarshal(c.ReferenceAssetIDsJSON, &refs); err != nil {
				return b, err
			}
		}
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			a, err := s.database.GetAsset(ref)
			if err != nil || a.ProjectID == nil || *a.ProjectID != project {
				return b, fmt.Errorf("scene reference is not in this project")
			}
			seen[ref] = true
			b.ReferenceAssetIDs = append(b.ReferenceAssetIDs, ref)
		}
	}
	if len(b.ReferenceAssetIDs) > 8 {
		return b, fmt.Errorf("scene has more than eight image references; reduce its cast references before generating")
	}
	return b, nil
}
