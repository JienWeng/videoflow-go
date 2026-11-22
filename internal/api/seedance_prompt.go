package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/models"
	"videoflow-go/internal/seedance"
)

// finalPromptKey holds the prompt the video service receives for a planned scene clip.
// It is written once from the scene's own records, in the form the Seedance guides ask
// for, and replaces the text earlier stages accumulate.
const finalPromptKey = "final_prompt"

// writeVideoPrompt asks the prompt-writing AI for one clip's prompt. Tests replace it.
var writeVideoPrompt = func(s *Server, ctx context.Context, system, facts string) (string, error) {
	var out struct {
		Prompt string `json:"prompt"`
	}
	err := s.agentJSON(ctx, "prompt_agent", system, facts, nil, &out)
	return out.Prompt, err
}

// writtenPrompt is the AI writer's prompt for a clip: it reads the playbook sections the
// clip needs and directs the shots, and its result is kept only when it leaves the
// scene's facts intact. The same clip is written once and remembered. With write off,
// only a remembered prompt is returned.
func (s *Server) writtenPrompt(ctx context.Context, clip seedance.Clip, write bool) (string, bool) {
	system, facts := seedance.Brief(clip)
	sum := sha256.Sum256([]byte(system + "\x00" + facts))
	key := "scene_prompt:" + hex.EncodeToString(sum[:])
	var saved string
	if raw, err := s.database.GetAppSetting(key); err == nil && json.Unmarshal([]byte(raw), &saved) == nil && seedance.Check(clip, saved) == nil {
		return saved, true
	}
	if !write {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	prompt, err := writeVideoPrompt(s, ctx, system, facts)
	if err == nil {
		err = seedance.Check(clip, prompt)
	}
	if err != nil && strings.TrimSpace(prompt) != "" {
		// One chance to repair: the writer is told exactly what was refused.
		refused := err.Error()
		if prompt, err = writeVideoPrompt(s, ctx, system, facts+"\n\nYour previous prompt was refused: "+refused+". Fix exactly that, keep everything else, and return the whole prompt again.\nPrevious prompt:\n"+prompt); err == nil {
			err = seedance.Check(clip, prompt)
		}
	}
	if err != nil {
		slog.Warn("video prompt writer fell back to the plain draft", "reason", err.Error())
		return "", false
	}
	prompt = strings.TrimSpace(prompt)
	raw, _ := json.Marshal(prompt)
	_ = s.database.SetAppSetting(key, string(raw))
	return prompt, true
}

// writeScenePrompt puts the prompt of a planned scene clip into its request: written by
// the AI prompt writer from the playbook when write is on (or already remembered), else
// the plain draft. Requests that are not a planned part of a scene keep their own prompt.
func (s *Server) writeScenePrompt(ctx context.Context, job *models.RenderJob, write bool) {
	unit := renderRequestUnit(job.RequestJSON)
	if job.SceneID == nil || len(unit.ShotIDs) == 0 {
		return
	}
	scene, err := s.database.GetScene(*job.SceneID)
	if err != nil {
		return
	}
	var payload map[string]any
	if json.Unmarshal(job.RequestJSON, &payload) != nil {
		return
	}
	brief, settings := sceneBrief(scene), sceneProduction(scene)
	provider, model := job.Provider, job.Model
	if provider == "" {
		provider, model = s.routes().MediaSelection("video")
	}
	style := s.sceneVisualStyle(scene)
	if style == defaultStyleGuide(*scene.ProjectID).StylePrompt {
		style = "" // no look was chosen; the placeholder is advice for a writer, not a look
	}
	clip := seedance.Clip{
		Family: costs.FamilyOf(provider, model), Style: style, Summary: scene.Summary, Tone: brief.Tone,
		Narration: brief.ConversationMode == "narration", Language: brief.Language, Audio: seedance.Silent,
	}
	if policy, _ := payload["voice_policy"].(map[string]any); policy["mode"] == "native" {
		clip.Audio = seedance.Native
	}
	clip.References = clipReferences(payload)
	clip.Voices = textValues(payload["reference_voices"])
	if clip.Music, _ = payload["music_prompt"].(string); clip.Music == "" {
		clip.Music = settings.MusicPrompt
	}

	names := map[string]string{}
	ids := models.IDList(scene.CharacterIDsJSON)
	for _, id := range append(ids, brief.CharacterIDs...) {
		character, e := s.database.GetCharacter(id)
		if e != nil || character == nil || names[id] != "" {
			continue
		}
		names[id] = character.Name
		voice := []string{}
		_ = json.Unmarshal(character.VoiceRulesJSON, &voice)
		clip.Cast = append(clip.Cast, seedance.Character{Name: character.Name, Look: character.Appearance, Voice: strings.Join(voice, ", "), Manner: character.Personality})
	}
	// Places and props connected to the scene or to one of this clip's shots.
	connected := append([]string{}, s.sceneBoundAssets(scene)...)
	for _, id := range unit.ShotIDs {
		if shot, e := s.database.GetShot(id); e == nil {
			used := models.IDList(shot.AssetIDsJSON)
			connected = append(connected, used...)
		}
	}
	seen := map[string]bool{}
	for _, id := range connected {
		asset, e := s.database.GetAsset(id)
		if e != nil || seen[id] || asset.Type == "storyboard" || asset.Type == "character_reference" || asset.Type == "output_video" {
			continue
		}
		seen[id] = true
		clip.Things = append(clip.Things, seedance.Thing{Kind: asset.Type, Name: asset.Name, Look: asset.Description})
	}

	text := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	place := map[string]int{} // shot id → its number in this clip
	for _, id := range unit.ShotIDs {
		if shot, e := s.database.GetShot(id); e == nil {
			clip.Shots = append(clip.Shots, seedance.Shot{Seconds: shot.Duration, Framing: text(shot.Camera), Move: text(shot.Movement), Action: shot.Prompt})
			place[id] = len(clip.Shots)
		}
	}
	spoken := sceneSpokenLines(scene)
	for _, line := range spoken {
		// A line pinned to a shot is spoken only in the clip that has that shot.
		target := settings.DialogueShots[line.ID]
		if target != "" && place[target] == 0 {
			continue
		}
		// Script writers sometimes save a code ("en"); the video model is told the language's name.
		language := line.Language
		if len(language) <= 3 && brief.Language != "" {
			language = brief.Language
		}
		clip.Lines = append(clip.Lines, seedance.Line{Speaker: names[line.CharacterID], Text: line.Text, Language: language, Shot: place[target]})
	}
	payload[finalPromptKey], payload["prompt_writer"] = seedance.Compose(clip), "template"
	payload["prompt_playbook"] = seedance.Sections(clip)
	if written, ok := s.writtenPrompt(ctx, clip, write); ok {
		payload[finalPromptKey], payload["prompt_writer"] = written, "ai"
	}
	job.RequestJSON = voiceJSON(payload)
}

// textValues reads a list of strings from a saved request.
func textValues(v any) []string {
	out := []string{}
	list, _ := v.([]any)
	for _, item := range list {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
