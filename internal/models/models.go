package models

import (
	"encoding/json"
	"time"
)

// Project table
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Character table
type Character struct {
	ID                    string          `json:"id"`
	ProjectID             *string         `json:"project_id"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	Appearance            string          `json:"appearance"`
	Personality           string          `json:"personality"`
	VisualRulesJSON       json.RawMessage `json:"visual_rules_json"`
	VoiceRulesJSON        json.RawMessage `json:"voice_rules_json"`
	SampleDialogue        string          `json:"sample_dialogue"`
	ReferenceAssetIDsJSON json.RawMessage `json:"reference_asset_ids_json"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// Asset table
type Asset struct {
	ID           string          `json:"id"`
	ProjectID    *string         `json:"project_id"`
	Type         string          `json:"type"`
	Name         string          `json:"name"`
	FilePath     string          `json:"file_path"`
	TagsJSON     json.RawMessage `json:"tags_json"`
	Description  string          `json:"description"`
	CharacterID  *string         `json:"character_id"`
	MetadataJSON json.RawMessage `json:"metadata_json"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Script table
type Script struct {
	ID        string          `json:"id"`
	ProjectID *string         `json:"project_id"`
	Idea      string          `json:"idea"`
	Title     string          `json:"title"`
	Summary   string          `json:"summary"`
	DraftJSON json.RawMessage `json:"draft_json"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Scene table
type Scene struct {
	ID               string          `json:"id"`
	ProjectID        *string         `json:"project_id"`
	ScriptID         *string         `json:"script_id"`
	SceneOrder       int             `json:"scene_order"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	Duration         int             `json:"duration"`
	AspectRatio      string          `json:"aspect_ratio"`
	CharacterIDsJSON json.RawMessage `json:"character_ids_json"`
	AssetIDsJSON     json.RawMessage `json:"asset_ids_json"`
	SceneJSON        json.RawMessage `json:"scene_json"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// Shot table
type Shot struct {
	ID           string          `json:"id"`
	SceneID      string          `json:"scene_id"`
	ShotOrder    int             `json:"shot_order"`
	Duration     int             `json:"duration"`
	Prompt       string          `json:"prompt"`
	Camera       *string         `json:"camera"`
	Movement     *string         `json:"movement"`
	AssetIDsJSON json.RawMessage `json:"asset_ids_json"`
	ShotJSON     json.RawMessage `json:"shot_json"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// RenderJob table
type RenderJob struct {
	ID            string          `json:"id"`
	ProjectID     *string         `json:"project_id"`
	SceneID       *string         `json:"scene_id"`
	ShotID        *string         `json:"shot_id"`
	Provider      string          `json:"provider"`
	Model         string          `json:"model"`
	ProviderJobID *string         `json:"provider_job_id"`
	Status        string          `json:"status"` // pending, running, succeeded, failed
	Stage         *string         `json:"stage"`
	Progress      *string         `json:"progress"`
	RequestJSON   json.RawMessage `json:"request_json"`
	ResponseJSON  json.RawMessage `json:"response_json"`
	Error         *string         `json:"error"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// RenderOutput table
type RenderOutput struct {
	ID            string          `json:"id"`
	RenderJobID   string          `json:"render_job_id"`
	VideoPath     string          `json:"video_path"`
	ThumbnailPath *string         `json:"thumbnail_path"`
	CaptionedPath *string         `json:"captioned_path"`
	CaptionsJSON  json.RawMessage `json:"captions_json"`
	Score         *float64        `json:"score"`
	QAJSON        json.RawMessage `json:"qa_json"`
	Selected      bool            `json:"selected"`
	Notes         string          `json:"notes"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// StyleGuide table
type StyleGuide struct {
	ID          string    `json:"id"`
	ProjectID   *string   `json:"project_id"`
	Name        string    `json:"name"`
	StylePrompt string    `json:"style_prompt"`
	Palette     string    `json:"palette"`
	Lighting    string    `json:"lighting"`
	Audience    string    `json:"audience"`
	Tone        string    `json:"tone"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Op table (for long-running operations)
type Op struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`   // video_generation, conversation, qa, etc.
	Status     string          `json:"status"` // running, succeeded, failed
	SceneID    *string         `json:"scene_id"`
	OutputID   *string         `json:"output_id"`
	ProjectID  *string         `json:"project_id"`
	Error      *string         `json:"error"`
	ResultJSON json.RawMessage `json:"result_json"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// AppSetting table
type AppSetting struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// AgentSetting table
type AgentSetting struct {
	Agent          string  `json:"agent"`
	ProjectID      *string `json:"project_id"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	PromptTemplate string  `json:"prompt_template"`
}

// Graph Response
type GraphNode struct {
	ID    string                 `json:"id"`
	Type  string                 `json:"type"`
	Label string                 `json:"label"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}

type GraphResponse struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}
