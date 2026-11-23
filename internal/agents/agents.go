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
	"videoflow-go/internal/speech"
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
Return a title, overall summary, and the list of scenes. For spoken dialogue/narration include spoken_lines with stable unique utterance id, character_id from the selected cast (empty for narrator), language, exact text, and visible indicating on-screen dialogue. Silent scenes have no spoken lines. Never use visual descriptions as spoken text.
%s`, sceneCount, targetDuration, aspectRatio, ScriptSpeechRules(a.dialogueLanguage(ctx)))
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
	a.shortenOverlongScenes(ctx, &res)
	return &res, nil
}

// dialogueLanguage is the language dialogue is written in, as structured() tells the writer.
func (a *AgentEngine) dialogueLanguage(ctx context.Context) string {
	if value, _ := ctx.Value(dialogueLanguageKey{}).(string); value != "" {
		return value
	}
	if a.language != nil {
		if l := a.language(); l != "" {
			return l
		}
	}
	return "English"
}

// ScriptSpeechRules tells a script writer how much dialogue a scene holds in a language:
// enough that the speakers talk through most of each scene (70–90 % of its seconds at a
// calm pace), never more, in short lines that each fit one shot.
func ScriptSpeechRules(language string) string {
	unit := "words"
	if speech.CharBased(language) {
		unit = "characters"
	}
	r := speech.RateFor(language, nil)
	rate := r.Words
	if unit == "characters" {
		rate = r.Chars
	}
	eight, ten, shot := speech.BudgetFor(8, language, nil), speech.BudgetFor(10, language, nil), speech.BudgetFor(3, language, nil)
	return fmt.Sprintf(`Spoken lines are spoken inside the scene's video, so write enough for the speakers to talk through most of each scene, and never more than fits: in total aim for 70-90%% of the scene's seconds at a calm pace, leaving a breath at the end. In %s that is about %.1f %s a second: an 8-second scene holds %d-%d %s in total, a 10-second scene %d-%d. Do not write one short line for a whole scene. Split the dialogue into short lines of one breath each (one sentence or two short clauses, at most about %d %s), so each line can be spoken within a single 2-4 second shot.`,
		language, rate, unit, eight.Low, eight.High, unit, ten.Low, ten.High, shot.High, unit)
}

// shortenOverlongScenes asks the writer once to shorten the dialogue of scenes whose
// lines cannot be said in the scene's seconds, and keeps a rewrite only when it is
// shorter. It never asks twice: what still does not fit is flagged before rendering.
func (a *AgentEngine) shortenOverlongScenes(ctx context.Context, script *ScriptDraft) {
	type overlong struct {
		Scene   int          `json:"scene"`
		Seconds int          `json:"seconds"`
		Target  string       `json:"target"`
		Lines   []SpokenLine `json:"spoken_lines"`
	}
	used := func(lines []SpokenLine) float64 {
		in := []speech.Line{}
		for _, l := range lines {
			in = append(in, speech.Line{ID: l.ID, Text: l.Text, Language: l.Language, Hint: -1})
		}
		f := speech.FitLines(in, 0, nil)
		return f.Used
	}
	language := a.dialogueLanguage(ctx)
	over := []overlong{}
	for i, scene := range script.Scenes {
		if len(scene.SpokenLines) > 0 && used(scene.SpokenLines) > float64(scene.Duration) {
			b := speech.BudgetFor(float64(scene.Duration), language, nil)
			over = append(over, overlong{Scene: i + 1, Seconds: scene.Duration, Target: fmt.Sprintf("%d-%d %s in total", b.Low, b.High, b.Unit), Lines: scene.SpokenLines})
		}
	}
	if len(over) == 0 {
		return
	}
	facts, _ := json.Marshal(over)
	var res struct {
		Scenes []struct {
			Scene int          `json:"scene"`
			Lines []SpokenLine `json:"spoken_lines"`
		} `json:"scenes"`
	}
	system := `These scenes have more dialogue than can be said in their seconds. Shorten ONLY their spoken lines to the target length: keep each line's id, speaker (character_id), language and visible flag, keep the meaning and order, cut words rather than lines, and keep every line short enough to say in one breath. Return JSON {"scenes": [{"scene": <number>, "spoken_lines": [...]}]} with every scene you were given.`
	if err := a.structured(ctx, "script_fit_agent", system, string(facts), &res); err != nil {
		return // the long lines stay; the speech budget flags them before any video is made
	}
	for _, fixed := range res.Scenes {
		i := fixed.Scene - 1
		if i < 0 || i >= len(script.Scenes) || len(fixed.Lines) == 0 {
			continue
		}
		before := script.Scenes[i].SpokenLines
		speakers := map[string]bool{}
		for _, l := range before {
			speakers[l.CharacterID] = true
		}
		ok := true
		for _, l := range fixed.Lines {
			ok = ok && strings.TrimSpace(l.Text) != "" && speakers[l.CharacterID]
		}
		if ok && used(fixed.Lines) < used(before) {
			script.Scenes[i].SpokenLines = fixed.Lines
		}
	}
}

// --- Generate Shots ---

type ShotDraft struct {
	Order    int    `json:"shot_order"`
	Duration int    `json:"duration"`
	Prompt   string `json:"prompt"`
	Camera   string `json:"camera"`
	Movement string `json:"movement"`
	// LineIDs are the scene's spoken lines said in this shot, when lines were given.
	LineIDs []string `json:"spoken_line_ids,omitempty"`
}

type ShotsListDraft struct {
	Shots []ShotDraft `json:"shots"`
}

// shotProposal accepts numeric AI timings without changing the integer timing
// contract used by saved shots, the editor and video providers.
type shotProposal struct {
	Order    int      `json:"shot_order"`
	Duration float64  `json:"duration"`
	Prompt   string   `json:"prompt"`
	Camera   string   `json:"camera"`
	Movement string   `json:"movement"`
	LineIDs  []string `json:"spoken_line_ids,omitempty"`
}

// ShotLine is a spoken line the shot planner places in a shot.
type ShotLine struct {
	ID      string  `json:"id"`
	Speaker string  `json:"speaker"`
	Text    string  `json:"text"`
	Seconds float64 `json:"seconds_to_say"`
}

// GenerateShots plans a scene's shots. With lines, each shot also says which of the
// scene's spoken lines are said in it, and is planned long enough to say them.
func (a *AgentEngine) GenerateShots(ctx context.Context, title, summary string, duration int, lines ...ShotLine) ([]ShotDraft, error) {
	if duration <= 0 {
		return nil, fmt.Errorf("scene duration must be positive")
	}
	system := fmt.Sprintf("You are a cinematographer. Break down the scene into %d to %d detailed camera shots (angles, movements, and visual prompts for video generation AI). Return shots in playback order with shot_order numbered from 1. Each shot must include a nonempty visual prompt. Use positive whole-second shot durations totaling exactly %d seconds.\n%s", min(2, duration), min(4, duration), duration, seedance.ShotRules)
	prompt := fmt.Sprintf("Scene Title: %s\nSummary: %s\nTotal Duration: %d seconds", title, summary, duration)
	if len(lines) > 0 {
		system += "\n- Spoken lines: the scene's lines are given with how many seconds each takes to say. Put each line in exactly one shot, in the given order, by listing its id in that shot's spoken_line_ids. Show the speaker of a character's line on screen in that shot. Give a shot with lines at least the seconds its lines take plus a breath (about 0.4 s between lines); never put more speech in a shot than its seconds hold."
		raw, _ := json.Marshal(lines)
		prompt += "\nSpoken lines (in order): " + string(raw)
	}
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
	known, placed := map[string]bool{}, map[string]bool{}
	for _, l := range lines {
		known[l.ID] = true
	}
	for i, shot := range res.Shots {
		// Each known line is kept once, in the first shot that names it.
		for _, id := range shot.LineIDs {
			if known[id] && !placed[id] {
				shots[i].LineIDs = append(shots[i].LineIDs, id)
				placed[id] = true
			}
		}
	}
	for i, shot := range res.Shots {
		ideal[i] = (shot.Duration / total) * float64(duration)
		// Reserve a second for every remaining shot before accumulating, so
		// minimum durations cannot push the total past the scene or overflow.
		seconds := duration
		if ideal[i] < float64(duration) {
			seconds = max(1, int(math.Floor(ideal[i])))
		}
		seconds = min(seconds, duration-used-(len(res.Shots)-i-1))
		shots[i] = ShotDraft{Order: i + 1, Duration: seconds, Prompt: shot.Prompt, Camera: shot.Camera, Movement: shot.Movement, LineIDs: shots[i].LineIDs}
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

// --- Fit Script To Shots ---

// FitShot is one shot a scene's spoken lines are fitted to.
type FitShot struct {
	Number  int    `json:"shot"`
	Seconds int    `json:"seconds"`
	Action  string `json:"what_happens"`
	// Target is how much speech the shot should hold ("8-10 characters"); Most is the
	// most it holds at all.
	Target string `json:"target_length"`
	Most   string `json:"holds_at_most"`
}

// FitLine is a spoken line as it is now, with the shot it is in and how long it takes.
type FitLine struct {
	ID          string  `json:"id"`
	CharacterID string  `json:"character_id"`
	Speaker     string  `json:"speaker"`
	Text        string  `json:"text"`
	Shot        int     `json:"shot"`
	Seconds     float64 `json:"seconds_to_say"`
}

// FittedLine is a rewritten line and the shot it is spoken in, counted from 1.
type FittedLine struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	Text        string `json:"text"`
	Shot        int    `json:"shot"`
}

// FitScript rewrites only a scene's spoken lines so each shot's lines fill most of its
// seconds and never more. One turn; the caller checks the answer.
func (a *AgentEngine) FitScript(ctx context.Context, title, summary, language string, shots []FitShot, lines []FitLine, speakers map[string]string) ([]FittedLine, error) {
	system := fmt.Sprintf(`You fit a scene's spoken script to its shots. Rewrite ONLY the spoken lines; the shots, their seconds and what happens in them stay as they are.
- Each shot's lines together should take 70-90%% of the shot's seconds at a calm pace (its target_length) and must never take more than holds_at_most. A shot may stay silent when the story wants a pause, but the scene as a whole should be spoken through most of its time; add words or a short line where a shot is nearly empty.
- Keep the language (%s), who speaks, the meaning and the order of the story. You may shorten, lengthen, split a line in two (the new part gets a new id: the old id with "_b"), merge two lines of the same speaker, or move a line to a neighbouring shot.
- Use only the speakers given (character_id; empty for the narrator). Never add a speaker.
- Lines are said one at a time, with a breath between them. Keep each line to one breath.
Return JSON {"lines": [{"id": "...", "character_id": "...", "text": "...", "shot": <number from 1>}]} in speaking order.`, language)
	facts := map[string]any{"scene": title, "summary": summary, "language": language, "speakers": speakers, "shots": shots, "lines": lines}
	return a.rewriteLines(ctx, "script_fit_agent", system, facts, shots, lines, speakers)
}

// RepairScript rewrites the spoken lines of one clip the speech check flagged (cut off,
// words changed, lines missing) shorter and simpler, so the video model can say them
// whole: heard are the check's findings in words. Only these lines are given; each
// stays with its speaker and in its shot. One turn; the caller checks the answer.
func (a *AgentEngine) RepairScript(ctx context.Context, title, summary, language string, shots []FitShot, lines []FitLine, speakers map[string]string, heard []string) ([]FittedLine, error) {
	system := fmt.Sprintf(`A video clip was made from these spoken lines, and listening to it showed the video model could not say them as written (see what_was_heard). Rewrite ONLY these lines SHORTER and SIMPLER so they are easy to say whole in the time.
- Each shot's lines together should take 60-75%% of the shot's seconds at a calm pace (its target_length) and must never take more than holds_at_most. Every line must come out shorter than it is now.
- Keep the language (%s), the meaning, the order, who speaks each line and the shot each line is in. Use plain everyday words, no lists, no names the video model could stumble on. Do not add, merge or split lines; keep every id.
- Use only the speakers given (character_id; empty for the narrator). Never add a speaker.
Return JSON {"lines": [{"id": "...", "character_id": "...", "text": "...", "shot": <number from 1>}]} in speaking order.`, language)
	facts := map[string]any{"scene": title, "summary": summary, "language": language, "speakers": speakers, "shots": shots, "lines": lines, "what_was_heard": heard}
	return a.rewriteLines(ctx, "speech_repair_agent", system, facts, shots, lines, speakers)
}

// rewriteLines asks the writer once and checks every line has text, a known speaker
// and a shot the scene has.
func (a *AgentEngine) rewriteLines(ctx context.Context, name, system string, facts map[string]any, shots []FitShot, lines []FitLine, speakers map[string]string) ([]FittedLine, error) {
	raw, _ := json.Marshal(facts)
	var res struct {
		Lines []FittedLine `json:"lines"`
	}
	if err := a.structured(ctx, name, system, string(raw), &res); err != nil {
		return nil, err
	}
	if len(res.Lines) == 0 && len(lines) > 0 {
		return nil, fmt.Errorf("the writer returned no spoken lines")
	}
	for i, l := range res.Lines {
		if strings.TrimSpace(l.Text) == "" {
			return nil, fmt.Errorf("line %d came back empty", i+1)
		}
		if _, ok := speakers[l.CharacterID]; !ok {
			return nil, fmt.Errorf("line %d is given to a speaker who is not in the scene", i+1)
		}
		if l.Shot < 1 || l.Shot > len(shots) {
			return nil, fmt.Errorf("line %d is placed in shot %d, which the scene does not have", i+1, l.Shot)
		}
	}
	return res.Lines, nil
}
