package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/models"
	"videoflow-go/internal/seedance"
)

// finalPromptKey holds the prompt the video service receives for a planned scene clip.
// It is written once from the scene's own records, in the form the Seedance guides ask
// for, and replaces the text earlier stages accumulate.
const finalPromptKey = "final_prompt"

// writeScenePrompt composes the prompt of a planned scene clip into its request.
// Requests that are not a planned part of a scene keep their own prompt.
func (s *Server) writeScenePrompt(job *models.RenderJob) {
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
	if clip.Music, _ = payload["music_prompt"].(string); clip.Music == "" {
		clip.Music = settings.MusicPrompt
	}

	names := map[string]string{}
	ids := []string{}
	_ = json.Unmarshal(scene.CharacterIDsJSON, &ids)
	for _, id := range append(ids, brief.CharacterIDs...) {
		character, e := s.database.GetCharacter(id)
		if e != nil || character == nil || names[id] != "" {
			continue
		}
		names[id] = character.Name
		voice := []string{}
		_ = json.Unmarshal(character.VoiceRulesJSON, &voice)
		clip.Cast = append(clip.Cast, seedance.Character{Name: character.Name, Look: character.Appearance, Voice: strings.Join(voice, ", ")})
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
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script)
	for _, line := range script.Lines {
		// A line pinned to a shot is spoken only in the clip that has that shot.
		target := settings.DialogueShots[line.ID]
		if target != "" && place[target] == 0 {
			continue
		}
		clip.Lines = append(clip.Lines, seedance.Line{Speaker: names[line.CharacterID], Text: line.Text, Language: line.Language, Shot: place[target]})
	}
	payload[finalPromptKey] = seedance.Compose(clip)
	job.RequestJSON = voiceJSON(payload)
}

// handleScenePromptPreview shows, without making or sending anything, the prompt each
// clip of a scene would be sent with right now.
func (s *Server) handleScenePromptPreview(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, e := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "that scene is not in the open project", 404)
		return
	}
	plan, e := s.sceneProductionPlan(r.Context(), scene)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	clips := []map[string]any{}
	for _, unit := range plan.Units {
		job := sceneRenderJob(project, scene)
		job.Provider, job.Model = plan.Provider, plan.Model
		request := map[string]any{"render_unit": unit, "music_prompt": plan.Settings.MusicPrompt, "voice_policy": map[string]any{"mode": plan.Settings.AudioMode}}
		job.RequestJSON = voiceJSON(request)
		s.writeScenePrompt(job)
		var written map[string]any
		_ = json.Unmarshal(job.RequestJSON, &written)
		prompt, _ := written[finalPromptKey].(string)
		clips = append(clips, map[string]any{"clip": unit.Index + 1, "seconds": unit.Duration, "prompt": prompt, "characters": len(prompt)})
	}
	writeJSON(w, 200, map[string]any{"scene_id": scene.ID, "model": plan.Model, "audio": plan.Settings.AudioMode, "clips": clips})
}
