package agents

import (
	"context"
	"encoding/json"
	"fmt"

	"videoflow-go/internal/providers"
)

type AgentEngine struct {
	openrouter *providers.OpenRouterClient
	model      string
}

func NewAgentEngine(openrouter *providers.OpenRouterClient, defaultModel string) *AgentEngine {
	if defaultModel == "" {
		defaultModel = "openai/gpt-4o-mini"
	}
	return &AgentEngine{
		openrouter: openrouter,
		model:      defaultModel,
	}
}

// --- Develop Idea ---

type ConceptOption struct {
	Title       string `json:"title"`
	Hook        string `json:"hook"`
	Narrative   string `json:"narrative"`
	VisualStyle string `json:"visual_style"`
}

type DevelopIdeaResponse struct {
	OptionA        ConceptOption `json:"option_a"`
	OptionB        ConceptOption `json:"option_b"`
	Recommendation string        `json:"recommendation"`
}

func (a *AgentEngine) DevelopIdea(ctx context.Context, idea string) (*DevelopIdeaResponse, error) {
	system := "You are a professional film and animation director. Expand the user's idea into two mature, distinct concept directions (Option A and Option B), and give a recommendation."
	prompt := fmt.Sprintf("Idea: %s", idea)

	var res DevelopIdeaResponse
	err := a.openrouter.StructuredJSON(ctx, a.model, system, prompt, &res)
	if err != nil {
		// Fallback deterministic option if offline or key missing
		return &DevelopIdeaResponse{
			OptionA: ConceptOption{
				Title:       "Cinematic Narrative",
				Hook:        "A captivating opening sequence that introduces the central conflict.",
				Narrative:   idea,
				VisualStyle: "Rich cinematic lighting with wide anamorphic framing.",
			},
			OptionB: ConceptOption{
				Title:       "Fast-Paced Action Story",
				Hook:        "Starts directly in the middle of a high-stakes scene.",
				Narrative:   idea + " with rapid scene shifts.",
				VisualStyle: "Dynamic handheld camera movement and vibrant contrast.",
			},
			Recommendation: "Option A offers deeper character development and visual consistency.",
		}, nil
	}
	return &res, nil
}

// --- Generate Script & Scenes ---

type SceneDraft struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Duration    int    `json:"duration"`
	AspectRatio string `json:"aspect_ratio"`
}

type ScriptDraft struct {
	Title   string       `json:"title"`
	Summary string       `json:"summary"`
	Scenes  []SceneDraft `json:"scenes"`
}

func (a *AgentEngine) GenerateScript(ctx context.Context, idea string, sceneCount int, targetDuration int, aspectRatio string) (*ScriptDraft, error) {
	if sceneCount <= 0 {
		sceneCount = 3
	}
	if targetDuration <= 0 {
		targetDuration = sceneCount * 5
	}
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	system := fmt.Sprintf(`You are a scriptwriter. Break down the story idea into exactly %d structured scenes.
Total target duration is %d seconds. Aspect ratio is %s.
Return a title, overall summary, and the list of scenes.`, sceneCount, targetDuration, aspectRatio)

	prompt := fmt.Sprintf("Idea: %s", idea)

	var res ScriptDraft
	err := a.openrouter.StructuredJSON(ctx, a.model, system, prompt, &res)
	if err != nil {
		// Deterministic fallback
		draft := &ScriptDraft{
			Title:   "Story: " + idea,
			Summary: idea,
			Scenes:  []SceneDraft{},
		}
		perScene := targetDuration / sceneCount
		if perScene < 3 {
			perScene = 5
		}
		for i := 1; i <= sceneCount; i++ {
			draft.Scenes = append(draft.Scenes, SceneDraft{
				Title:       fmt.Sprintf("Scene %d", i),
				Summary:     fmt.Sprintf("Progression part %d for: %s", i, idea),
				Duration:    perScene,
				AspectRatio: aspectRatio,
			})
		}
		return draft, nil
	}
	return &res, nil
}

// --- Generate Shots ---

type ShotDraft struct {
	Order    int    `json:"shot_order"`
	Duration int    `json:"duration"`
	Prompt   string `json:"prompt"`
	Camera   string `json:"camera"`
	Movement string `json:"movement"`
}

type ShotsListDraft struct {
	Shots []ShotDraft `json:"shots"`
}

func (a *AgentEngine) GenerateShots(ctx context.Context, title, summary string, duration int) ([]ShotDraft, error) {
	system := "You are a cinematographer. Break down the scene into 2 to 4 detailed camera shots (angles, movements, and visual prompts for video generation AI)."
	prompt := fmt.Sprintf("Scene Title: %s\nSummary: %s\nTotal Duration: %d seconds", title, summary, duration)

	var res ShotsListDraft
	err := a.openrouter.StructuredJSON(ctx, a.model, system, prompt, &res)
	if err != nil || len(res.Shots) == 0 {
		// Deterministic fallback
		dur1 := duration / 2
		if dur1 <= 0 {
			dur1 = 3
		}
		dur2 := duration - dur1
		if dur2 <= 0 {
			dur2 = 3
		}
		return []ShotDraft{
			{
				Order:    1,
				Duration: dur1,
				Prompt:   fmt.Sprintf("Establishing shot of %s, cinematic lighting", summary),
				Camera:   "wide",
				Movement: "slow pan",
			},
			{
				Order:    2,
				Duration: dur2,
				Prompt:   fmt.Sprintf("Close up dramatic focus on the subject in %s", summary),
				Camera:   "close-up",
				Movement: "static",
			},
		}, nil
	}
	return res.Shots, nil
}

// --- Classify Chat Intent ---

type ChatIntentResponse struct {
	Reply        string                 `json:"reply"`
	Action       string                 `json:"action"` // create_script, add_scene, render_video, none
	ActionParams map[string]interface{} `json:"action_params,omitempty"`
}

func (a *AgentEngine) Chat(ctx context.Context, message string, history []map[string]string) (*ChatIntentResponse, error) {
	system := `You are the VideoFlow Studio assistant. Help the user direct their story, build characters, scenes, and shots.
Respond in JSON with a natural conversational 'reply', and an optional 'action' (create_script, add_scene, none).`

	historyBytes, _ := json.Marshal(history)
	prompt := fmt.Sprintf("Conversation history: %s\nUser message: %s", string(historyBytes), message)

	var res ChatIntentResponse
	err := a.openrouter.StructuredJSON(ctx, a.model, system, prompt, &res)
	if err != nil {
		return &ChatIntentResponse{
			Reply:  "I understand! Let's work on developing this scene and preparing the video shots.",
			Action: "none",
		}, nil
	}
	return &res, nil
}
