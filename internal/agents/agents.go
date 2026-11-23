package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"videoflow-go/internal/presets"
	"videoflow-go/internal/seedance"
)

// AgentEngine writes the app's text: every call is answered by the selected local
// session through the runner the server supplies.
type AgentEngine struct {
	structuredRunner func(context.Context, string, string, string, any) error
	language         func() string
}

func NewAgentEngine() *AgentEngine { return &AgentEngine{} }

type dialogueLanguageKey struct{}
type storyCharactersKey struct{}

func WithStoryCharacters(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, storyCharactersKey{}, enabled)
}

func WithDialogueLanguage(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, dialogueLanguageKey{}, language)
}
func (a *AgentEngine) SetLanguageResolver(resolve func() string) { a.language = resolve }

func (a *AgentEngine) SetStructuredRunner(runner func(context.Context, string, string, string, any) error) {
	a.structuredRunner = runner
}
func (a *AgentEngine) structured(ctx context.Context, name, system, prompt string, target any) error {
	if direction, _ := ctx.Value(creativeDirectionKey{}).(string); direction != "" && !strings.Contains(prompt, direction) {
		system += "\n" + direction
	}
	stage := ""
	if name == "script_agent" {
		stage = "story"
	}
	if name == "shot_agent" {
		stage = "shots"
	}
	if stage != "" {
		prompt += presets.Prompt(presets.From(ctx), stage)
	}
	if a.structuredRunner == nil {
		return fmt.Errorf("no local AI session is available to write this")
	}
	language := "English"
	if a.language != nil {
		language = a.language()
	}
	if value, _ := ctx.Value(dialogueLanguageKey{}).(string); value != "" {
		language = value
	}
	return a.structuredRunner(ctx, name, system+"\nWrite all character dialogue in "+language+".", prompt, target)
}

// --- Develop Idea ---

type ConceptOption struct {
	Title      string `json:"title"`
	Premise    string `json:"premise"`
	Hook       string `json:"hook"`
	WhyItWorks string `json:"why_it_works"`
}
type DevelopIdeaResponse struct {
	Options          []ConceptOption `json:"options"`
	RecommendedIndex int             `json:"recommended_index"`
	Reasoning        string          `json:"reasoning"`
}

// UnmarshalJSON accepts old provider replies without advertising the legacy schema to new calls.
func (response *DevelopIdeaResponse) UnmarshalJSON(data []byte) error {
	type canonical DevelopIdeaResponse
	var current canonical
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}
	*response = DevelopIdeaResponse(current)
	if len(response.Options) > 0 {
		return nil
	}
	var legacy struct {
		OptionA, OptionB struct{ Title, Hook, Narrative string }
		Recommendation   string
	}
	// Legacy snake-case names are explicit; unknown historic fields are ignored.
	var raw struct {
		A              struct{ Title, Hook, Narrative string } `json:"option_a"`
		B              struct{ Title, Hook, Narrative string } `json:"option_b"`
		Recommendation string                                  `json:"recommendation"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	legacy.OptionA = raw.A
	legacy.OptionB = raw.B
	legacy.Recommendation = raw.Recommendation
	if legacy.OptionA.Title != "" && legacy.OptionB.Title != "" {
		response.Options = []ConceptOption{{Title: legacy.OptionA.Title, Premise: legacy.OptionA.Narrative, Hook: legacy.OptionA.Hook}, {Title: legacy.OptionB.Title, Premise: legacy.OptionB.Narrative, Hook: legacy.OptionB.Hook}}
		response.Reasoning = legacy.Recommendation
		if strings.EqualFold(strings.TrimSpace(legacy.Recommendation), "B") {
			response.RecommendedIndex = 1
		}
	}
	return nil
}
func (a *AgentEngine) DevelopIdea(ctx context.Context, idea string) (*DevelopIdeaResponse, error) {
	var response DevelopIdeaResponse
	if err := a.structured(ctx, "idea_agent", "Develop two distinct film concepts. Return JSON options (exactly two objects with title, premise, hook, why_it_works), recommended_index (0 or 1), and reasoning.", "Idea: "+idea, &response); err != nil {
		return nil, err
	}
	if len(response.Options) != 2 || response.RecommendedIndex < 0 || response.RecommendedIndex >= 2 {
		return nil, fmt.Errorf("Text engine returned invalid idea options")
	}
	for _, o := range response.Options {
		if strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Premise) == "" {
			return nil, fmt.Errorf("Text engine returned incomplete idea options")
		}
	}
	return &response, nil
}

// --- Generate Script & Scenes ---

type SpokenLine struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	Language    string `json:"language"`
	Text        string `json:"text"`
	Visible     bool   `json:"visible"`
}
type SceneDraft struct {
	CharacterIDs []string     `json:"character_ids"`
	SpokenLines  []SpokenLine `json:"spoken_lines"`
	Title        string       `json:"title"`
	Summary      string       `json:"summary"`
	Duration     int          `json:"duration"`
	AspectRatio  string       `json:"aspect_ratio"`
}

type ScriptDraft struct {
	Characters []CharacterDraft `json:"characters"`
	Title      string           `json:"title"`
	Summary    string           `json:"summary"`
	Scenes     []SceneDraft     `json:"scenes"`
}

type CharacterDraft struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Appearance  string `json:"appearance"`
	Personality string `json:"personality"`
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
Return a title, overall summary, and the list of scenes. For spoken dialogue/narration include spoken_lines with stable unique utterance id, character_id from the selected cast (empty for narrator), language, exact text, and visible indicating on-screen dialogue. Silent scenes have no spoken lines. Never use visual descriptions as spoken text. Spoken lines must fit their scene when read at a calm narrator pace: in total at most about 2.2 words per second of the scene's duration (about 22 English words for a 10-second scene, or the same speaking time in other languages), leaving a breath at the end.`, sceneCount, targetDuration, aspectRatio)
	if enabled, _ := ctx.Value(storyCharactersKey{}).(bool); enabled {
		system += `
Create character profiles only for additional actors the story actually needs. Return them in characters with a unique short ASCII key (letters, digits, underscore or hyphen), name, description, consistent visual appearance, and personality. Reuse selected existing characters by their exact IDs; never redefine their appearance. Do not invent database IDs. Refer to a new character as new:key in each scene's character_ids and spoken_lines.character_id. Each scene lists only the characters it uses; the narrator has an empty character_id. Keep the total cast to at most eight. Do not create extra portraits, voice profiles or unrelated characters. If no new characters are needed, characters is empty.`
	} else {
		system += "\nDo not create character profiles: characters must be empty. Scene character_ids and spoken_lines.character_id may use only the selected cast's exact IDs, or an empty narrator ID."
	}

	prompt := fmt.Sprintf("Idea: %s", idea)

	var res ScriptDraft
	err := a.structured(ctx, "script_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	if len(res.Scenes) != sceneCount {
		return nil, fmt.Errorf("Text engine returned an unexpected scene count")
	}
	for i := range res.Scenes {
		scene := &res.Scenes[i]
		if scene.Title == "" || scene.Summary == "" || scene.Duration <= 0 {
			return nil, fmt.Errorf("Text engine returned an incomplete scene")
		}
		if scene.AspectRatio == "" {
			scene.AspectRatio = aspectRatio
		}
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

// shotProposal accepts numeric AI timings without changing the integer timing
// contract used by saved shots, the editor and video providers.
type shotProposal struct {
	Order    int     `json:"shot_order"`
	Duration float64 `json:"duration"`
	Prompt   string  `json:"prompt"`
	Camera   string  `json:"camera"`
	Movement string  `json:"movement"`
}

func (a *AgentEngine) GenerateShots(ctx context.Context, title, summary string, duration int) ([]ShotDraft, error) {
	if duration <= 0 {
		return nil, fmt.Errorf("scene duration must be positive")
	}
	system := fmt.Sprintf("You are a cinematographer. Break down the scene into %d to %d detailed camera shots (angles, movements, and visual prompts for video generation AI). Return shots in playback order with shot_order numbered from 1. Each shot must include a nonempty visual prompt. Use positive whole-second shot durations totaling exactly %d seconds.\n%s", min(2, duration), min(4, duration), duration, seedance.ShotRules)
	prompt := fmt.Sprintf("Scene Title: %s\nSummary: %s\nTotal Duration: %d seconds", title, summary, duration)
	var res struct {
		Shots []shotProposal `json:"shots"`
	}
	if err := a.structured(ctx, "shot_agent", system, prompt, &res); err != nil {
		return nil, err
	}
	if len(res.Shots) == 0 {
		return nil, fmt.Errorf("Text engine returned no shots")
	}
	if len(res.Shots) > duration {
		return nil, fmt.Errorf("Text engine returned more shots than fit in the scene's whole-second duration")
	}
	total := 0.0
	for i, shot := range res.Shots {
		if strings.TrimSpace(shot.Prompt) == "" {
			return nil, fmt.Errorf("Shot %d is missing its visual prompt. Generate shots again or add the shot manually in the scene studio.", i+1)
		}
		if shot.Duration <= 0 || math.IsNaN(shot.Duration) || math.IsInf(shot.Duration, 0) {
			return nil, fmt.Errorf("Shot %d has an invalid duration. Generate shots again using positive durations in seconds.", i+1)
		}
		total += shot.Duration
	}
	if math.IsInf(total, 0) {
		return nil, fmt.Errorf("Text engine returned invalid shot durations")
	}
	// Array order is the playback order. Number locally: omitted, duplicated or
	// sparse model numbering must not discard an otherwise valid shot plan.
	shots := make([]ShotDraft, len(res.Shots))
	ideal := make([]float64, len(shots))
	used := 0
	for i, shot := range res.Shots {
		ideal[i] = (shot.Duration / total) * float64(duration)
		// Reserve a second for every remaining shot before accumulating, so
		// minimum durations cannot push the total past the scene or overflow.
		seconds := duration
		if ideal[i] < float64(duration) {
			seconds = max(1, int(math.Floor(ideal[i])))
		}
		seconds = min(seconds, duration-used-(len(res.Shots)-i-1))
		shots[i] = ShotDraft{Order: i + 1, Duration: seconds, Prompt: shot.Prompt, Camera: shot.Camera, Movement: shot.Movement}
		used += seconds
	}
	// Largest-deficit rounding keeps the exact scene total, gives each shot at
	// least one second, and resolves equal remainders in original shot order.
	for used < duration {
		best := 0
		gap := math.Inf(-1)
		for i, shot := range shots {
			candidate := ideal[i] - float64(shot.Duration)
			if candidate > gap {
				best = i
				gap = candidate
			}
		}
		shots[best].Duration++
		used++
	}
	return shots, nil
}

// --- Classify Chat Intent ---

type ChatIntentResponse struct {
	CreationBrief *CreativeBrief `json:"creation_brief,omitempty"`
	AssetType     string         `json:"asset_type,omitempty"`
	Reply         string         `json:"reply"`
	Action        string         `json:"action"` // canonical action-card action, or none
	SceneCount    int            `json:"scene_count,omitempty"`
	SceneID       string         `json:"scene_id,omitempty"`
	CharacterID   string         `json:"character_id,omitempty"`
	// CharacterIDs names the characters generate_character_references draws; empty means
	// every character without a reference picture.
	CharacterIDs []string `json:"character_ids,omitempty"`
	OutputID     string   `json:"output_id,omitempty"`
	EditorPanel  string   `json:"editor_panel,omitempty"`
	// UserLanguage is the ISO 639-1 code of the language the user writes in.
	UserLanguage     string            `json:"user_language,omitempty"`
	CharacterChanges *CharacterChanges `json:"character_changes,omitempty"`
	// EpisodeNumber names the episode a finish_video or render_scenes request is about (0 = newest).
	EpisodeNumber int `json:"episode_number,omitempty"`
	// SceneIDs names the scenes render_scenes makes; empty means every scene of the
	// episode that has no video yet.
	SceneIDs []string `json:"scene_ids,omitempty"`
	// JobID is the waiting video request check_video or stop_waiting settles.
	JobID string `json:"job_id,omitempty"`
	// SwitchTo names the video service switch_video_service moves to: an id from
	// video_service.can_switch_to, or "cheapest".
	SwitchTo string `json:"switch_to,omitempty"`
	// Music is the music direction set_music saves on an episode ("none" removes it).
	Music string `json:"music,omitempty"`
	// NextEpisode marks a new story that continues this project's series.
	NextEpisode bool `json:"next_episode,omitempty"`
	// Instruction is the planner's own complete direction for the button it offers.
	Instruction string `json:"instruction,omitempty"`
	// Decision says whether the planner acts, asks or only informs, with the
	// assumptions it made. The server validates it (internal/api/chat_decision.go).
	Decision *ChatDecision `json:"decision,omitempty"`
	// Remember is what the planner wants kept in the project's decision log.
	Remember []RememberItem `json:"remember,omitempty"`
}

// ChatDecision is the planner's ask-or-decide choice for one turn.
type ChatDecision struct {
	// Mode is "act" (go ahead with the action), "ask" (one question the user must
	// answer first) or "inform" (an answer, no step to take).
	Mode string `json:"mode"`
	// Assumptions are the choices made without asking, each one short line.
	Assumptions []string `json:"assumptions,omitempty"`
	// Question and Options are set only to ask; options become quick replies.
	Question string           `json:"question,omitempty"`
	Options  []DecisionOption `json:"options,omitempty"`
	// Reason is why asking is needed: "missing_input", "expensive", "options",
	// "confirm_plan" or "destructive".
	Reason string `json:"reason,omitempty"`
	// Key is the decision-log key the answer settles, such as "aspect_ratio".
	Key string `json:"key,omitempty"`
}

// DecisionOption is one quick reply to a question: Label is shown, Value is sent.
type DecisionOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// RememberItem is one decision-log entry the planner asks to keep.
type RememberItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CharacterChanges is the full rewritten text of each saved character field the user wants changed.
type CharacterChanges struct {
	Appearance  string `json:"appearance,omitempty"`
	Description string `json:"description,omitempty"`
	Personality string `json:"personality,omitempty"`
}

// chatSystemPrompt is the assistant's standing instructions, kept in its own file so
// changes to it can be read and reviewed.
//
//go:embed chat_prompt.md
var chatSystemPrompt string

func (a *AgentEngine) Chat(ctx context.Context, message string, history []map[string]string, projectContext ...string) (*ChatIntentResponse, error) {
	system := chatSystemPrompt

	system += "\n\n" + StudioRecovery
	historyBytes, _ := json.Marshal(history)
	prompt := fmt.Sprintf("Conversation history: %s\nUser message: %s\nProject context: %s", string(historyBytes), message, strings.Join(projectContext, "\n"))

	var res ChatIntentResponse
	err := a.structured(ctx, "intent_agent", system, prompt, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}
