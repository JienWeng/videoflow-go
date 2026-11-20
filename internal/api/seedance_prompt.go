package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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
		clip.Cast = append(clip.Cast, seedance.Character{Name: character.Name, Look: character.Appearance, Voice: strings.Join(voice, ", "), Manner: character.Personality})
	}
	// Places and props connected to the scene or to one of this clip's shots.
	connected := append([]string{}, s.sceneBoundAssets(scene)...)
	for _, id := range unit.ShotIDs {
		if shot, e := s.database.GetShot(id); e == nil {
			used := []string{}
			_ = json.Unmarshal(shot.AssetIDsJSON, &used)
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
	payload[finalPromptKey], payload["prompt_writer"] = seedance.Compose(clip), "template"
	payload["prompt_playbook"] = seedance.Sections(clip)
	if written, ok := s.writtenPrompt(ctx, clip, write); ok {
		payload[finalPromptKey], payload["prompt_writer"] = written, "ai"
	}
	job.RequestJSON = voiceJSON(payload)
}

// handleScenePromptPreview shows, without making or sending any video, the prompt each
// clip of a scene would be sent with right now. With ?write=1 the AI prompt writer writes
// it now (free of video credits) so it can be read before any video is made.
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
		s.writeScenePrompt(r.Context(), job, r.URL.Query().Get("write") == "1")
		var written map[string]any
		_ = json.Unmarshal(job.RequestJSON, &written)
		prompt, _ := written[finalPromptKey].(string)
		clips = append(clips, map[string]any{"clip": unit.Index + 1, "seconds": unit.Duration, "prompt": prompt, "characters": len(prompt), "written_by": written["prompt_writer"], "playbook": written["prompt_playbook"]})
	}
	writeJSON(w, 200, map[string]any{"scene_id": scene.ID, "model": plan.Model, "audio": plan.Settings.AudioMode, "clips": clips})
}
