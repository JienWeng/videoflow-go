package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
	"videoflow-go/internal/presets"
)

type generationBrief struct {
	StorytellingID     string          `json:"storytelling_id,omitempty"`
	BriefVersion       int             `json:"brief_version,omitempty"`
	BriefConfirmed     bool            `json:"brief_confirmed,omitempty"`
	StyleFingerprint   string          `json:"style_fingerprint,omitempty"`
	Tone               string          `json:"tone,omitempty"`
	Audience           string          `json:"audience,omitempty"`
	Platform           string          `json:"platform,omitempty"`
	Resolution         string          `json:"resolution,omitempty"`
	ReviewBeforeRender bool            `json:"review_before_render,omitempty"`
	PresetID           string          `json:"style_preset_id,omitempty"`
	CreativePreset     *presets.Preset `json:"creative_preset,omitempty"`
	Idea               string          `json:"idea"`
	TargetDuration     int             `json:"target_duration"`
	SceneCount         int             `json:"scene_count"`
	Style              string          `json:"style"`
	AspectRatio        string          `json:"aspect_ratio"`
	Language           string          `json:"language"`
	ConversationMode   string          `json:"conversation_mode"`
	Instruction        string          `json:"instruction"`
	CharacterIDs       []string        `json:"character_ids"`
	CreateCharacters   bool            `json:"create_characters"`
	CharacterNotes     string          `json:"character_notes,omitempty"`
	ReferenceAssetIDs  []string        `json:"reference_asset_ids,omitempty"`
}

func (s *Server) effectiveStyle(projectID, choice string) (string, bool) {
	if style, err := s.database.GetStyleGuide(projectID); err == nil && strings.TrimSpace(style.StylePrompt) != "" {
		return style.StylePrompt + ". Palette: " + style.Palette + ". Lighting: " + style.Lighting, true
	}
	if choice == "cinematic" {
		return "cinematic composition and lighting", false
	}
	return "warm 2D children's picture-book illustration, clean shapes, expressive faces", false
}
func (s *Server) prepareBrief(project string, b *generationBrief) error {
	if _, err := presets.StorytellingDirection(b.StorytellingID); err != nil {
		return err
	}
	if err := s.confirmedCreativeBrief(project, b); err != nil {
		return err
	}
	if strings.TrimSpace(b.Idea) == "" || len(b.Idea) > 20000 || len(b.Instruction) > 5000 {
		return fmt.Errorf("a story idea of at most 20000 characters is required")
	}
	if b.SceneCount < 0 || b.TargetDuration < 0 {
		return fmt.Errorf("scene count and duration must be positive")
	}
	if len(b.CharacterIDs) > 8 {
		return fmt.Errorf("choose at most 8 characters")
	}
	if b.SceneCount <= 0 {
		b.SceneCount = 3
	}
	if b.SceneCount > 20 {
		return fmt.Errorf("choose at most 20 scenes")
	}
	if b.TargetDuration <= 0 {
		b.TargetDuration = b.SceneCount * int(s.routes().NumberSetting("default_scene_duration", 5))
	}
	if b.TargetDuration < b.SceneCount {
		return fmt.Errorf("allow at least one second per scene")
	}
	if b.TargetDuration > 600 {
		return fmt.Errorf("target duration exceeds 600 seconds")
	}
	if b.AspectRatio == "" {
		b.AspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	if b.Language == "" {
		b.Language = s.routes().Setting("dialogue_language", "English")
	}
	if b.ConversationMode == "" {
		b.ConversationMode = "dialogue"
	}
	if b.ConversationMode != "dialogue" && b.ConversationMode != "narration" && b.ConversationMode != "silent" {
		return fmt.Errorf("unsupported dialogue mode")
	}
	b.CreativePreset = nil // Resolve trusted catalog data, never accept a client-supplied snapshot.
	if b.PresetID != "" && b.PresetID != "project-style" {
		p, err := s.findPreset(b.PresetID)
		if err != nil {
			return err
		}
		b.CreativePreset = p
		b.Style = p.Style
	} else {
		b.Style, _ = s.effectiveStyle(project, b.Style)
	}
	raw, _ := json.Marshal(b.CharacterIDs)
	notes, err := s.characterNotes(&project, raw)
	if err != nil {
		return err
	}
	b.CharacterNotes = notes
	b.ReferenceAssetIDs = nil
	seen := map[string]bool{}
	for _, id := range b.CharacterIDs {
		c, _ := s.database.GetCharacter(id)
		var ids []string
		json.Unmarshal(c.ReferenceAssetIDsJSON, &ids)
		for _, assetID := range ids {
			if !seen[assetID] && len(b.ReferenceAssetIDs) < 8 {
				a, err := s.database.GetAsset(assetID)
				if err != nil || a.ProjectID == nil || *a.ProjectID != project {
					return fmt.Errorf("character reference does not belong to this project")
				}
				seen[assetID] = true
				b.ReferenceAssetIDs = append(b.ReferenceAssetIDs, assetID)
			}
		}
	}
	return nil
}

// prepareSavedBrief validates server-persisted scenes without resolving a newer catalog.
func (s *Server) prepareSavedBrief(project string, b *generationBrief) error {
	snapshot, id, style := b.CreativePreset, b.PresetID, b.Style
	version := b.BriefVersion
	b.BriefVersion = 0
	defer func() { b.BriefVersion = version }()
	b.PresetID = ""
	if err := s.prepareBrief(project, b); err != nil {
		return err
	}
	b.CreativePreset = snapshot
	b.PresetID = id
	if style != "" {
		b.Style = style
	}
	return nil
}
func (b generationBrief) prompt() string {
	text := fmt.Sprintf("%s\nVisual style: %s\nSpoken language: %s\nMode: %s\nDirection: %s\n%s", b.Idea, b.Style, b.Language, b.ConversationMode, b.Instruction, b.CharacterNotes)
	if direction := b.creativeDirection(); direction != "" {
		text += "\n" + direction
	}
	return text
}

func (b generationBrief) creativeDirection() string {
	storytelling, _ := presets.StorytellingDirection(b.StorytellingID)
	if b.BriefVersion != 1 {
		return storytelling
	}
	return storytelling + "\n" + fmt.Sprintf("Confirmed story-specific creative direction: visual style=%s; tone=%s; audience=%s; destination platform=%s; format=%s; resolution=%s; total duration=%ds; scenes=%d. Preserve these choices over project defaults while keeping locked character/object identities. Platform is a delivery target, not authorization to publish.", b.Style, b.Tone, b.Audience, b.Platform, b.AspectRatio, b.Resolution, b.TargetDuration, b.SceneCount)
}
func (b generationBrief) context(ctx context.Context) context.Context {
	ctx = agents.WithCreativeDirection(ctx, b.creativeDirection())
	return presets.With(agents.WithStoryCharacters(agents.WithDialogueLanguage(ctx, b.Language), b.CreateCharacters), b.CreativePreset)
}
func (s *Server) persistDraft(project string, b generationBrief, draft *agents.ScriptDraft) (*models.Script, []models.Scene, error) {
	if err := applyConfirmedDraft(b, draft); err != nil {
		return nil, nil, err
	}
	characters, bindings, err := s.prepareStoryCharacters(project, b, draft)
	if err != nil {
		return nil, nil, err
	}
	raw, _ := json.Marshal(draft)
	if len(bindings) > 0 {
		var document map[string]any
		json.Unmarshal(raw, &document)
		document["character_bindings"] = bindings
		raw, _ = json.Marshal(document)
	}
	script := &models.Script{ProjectID: &project, Idea: b.Idea, Title: draft.Title, Summary: draft.Summary, DraftJSON: raw}

	scenes := []models.Scene{}
	for i, sc := range draft.Scenes {
		sceneBrief, err := s.storySceneBrief(project, b, sc.CharacterIDs, characters)
		if err != nil {
			return nil, nil, err
		}
		cast, _ := json.Marshal(sc.CharacterIDs)
		if string(cast) == "null" {
			cast = []byte("[]")
		}
		snapshot, _ := json.Marshal(map[string]any{"generation_brief": sceneBrief, "spoken_lines": sc.SpokenLines})
		scenes = append(scenes, models.Scene{ProjectID: &project, SceneOrder: i + 1, Title: sc.Title, Summary: sc.Summary, Duration: sc.Duration, AspectRatio: sc.AspectRatio, CharacterIDsJSON: cast, SceneJSON: snapshot})
	}
	if err := s.database.SaveStorySnapshot(script, scenes, characters); err != nil {
		return nil, nil, err
	}
	return script, scenes, nil
}
func sceneBrief(scene *models.Scene) generationBrief {
	var snapshot struct {
		Brief generationBrief `json:"generation_brief"`
	}
	json.Unmarshal(scene.SceneJSON, &snapshot)
	return snapshot.Brief
}
