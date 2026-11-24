package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

// episodeSound tells the assistant, per episode, where the voice comes from and what
// music was asked for, so it can answer "where is the music?" from facts.
func (s *Server) episodeSound(project string) []map[string]any {
	scenes, _ := s.database.ListScenes(project)
	out := []map[string]any{}
	for _, e := range s.seriesEpisodes(project) {
		voice, music, speech := "", "", ""
		for i := range scenes {
			scene := &scenes[i]
			if scene.ScriptID == nil || *scene.ScriptID != e.ScriptID {
				continue
			}
			settings, brief := sceneProduction(scene), sceneBrief(scene)
			speech = brief.ConversationMode
			switch settings.AudioMode {
			case "silent":
				voice = "silent"
			default:
				voice = "video model"
			}
			if music = settings.MusicPrompt; music == "" {
				music = brief.Music
			}
		}
		if music == "" {
			music = "none"
		}
		out = append(out, map[string]any{"episode_number": e.Number, "title": e.Title, "speech": speech, "voice": voice, "music": music})
	}
	return out
}

// chatSetMusic saves a music direction on every scene of one episode. The video model
// makes music together with the voice, so the scenes it returns are the ones whose
// videos must be made again to hear it.
func (s *Server) chatSetMusic(ctx context.Context, scenes []models.Scene, episodes []seriesEpisode, res *agents.ChatIntentResponse) ([]map[string]any, string) {
	music := strings.TrimSpace(res.Music)
	if music == "" {
		return nil, "Tell me what kind of music you want, for example “slow sad piano”, and I'll set it."
	}
	if lower := strings.ToLower(music); lower == "none" || lower == "no music" {
		music = ""
	}
	if len(music) > 1500 {
		music = music[:1500]
	}
	script := chatEpisodeScript(scenes, episodes, res)
	number := 0
	for _, e := range episodes {
		if e.ScriptID == script {
			number = e.Number
		}
	}
	project := ""
	for i := range scenes {
		if scenes[i].ScriptID != nil && *scenes[i].ScriptID == script && scenes[i].ProjectID != nil {
			project = *scenes[i].ProjectID
		}
	}
	if plan, clips := s.storySound(ctx, project, script); project != "" && clips > 1 {
		// Several clips: the music is one bed laid under the whole film in the edit, so
		// no video is made again (sound_plan.go).
		plan.Music, plan.DecidedBy = soundMusic{Mode: musicBed, Prompt: music}, decidedOwner
		if music == "" {
			plan.Music.Mode = musicNone
		}
		if _, err := s.saveSoundPlan(ctx, project, script, plan); err != nil {
			return nil, "I could not set the music: " + err.Error() + ". You can still upload a track in Edit video under Audio."
		}
		if music == "" {
			return nil, fmt.Sprintf("Music is now off for episode %d. Its clips keep their own voices and ambience, so no video is made again; the next time the scenes are joined there is no music bed.", number)
		}
		return nil, fmt.Sprintf("Music for episode %d is now “%s”: one track under the whole film, lowered while people speak. Its %d clips keep their own voices and ambience and make no music, so nothing is made again and it costs nothing. Say “finish the video” to join the scenes with it, or upload your own track in Edit video under Audio.", number, music, clips)
	}
	batch := []map[string]any{}
	for i := range scenes {
		scene := &scenes[i]
		if scene.ScriptID == nil || *scene.ScriptID != script {
			continue
		}
		settings := sceneProduction(scene)
		if settings.AudioMode == "silent" {
			return nil, fmt.Sprintf("“%s” is set to no sound, so the video model makes no music for it. Open Edit video and upload a track under Audio instead; that is free.", scene.Title)
		}
		settings.AudioMode, settings.MusicPrompt = "native", music
		var doc map[string]any
		if json.Unmarshal(scene.SceneJSON, &doc) != nil || doc == nil {
			doc = map[string]any{}
		}
		doc["production"] = settings
		scene.SceneJSON = voiceJSON(doc)
		if _, err := s.sceneProductionPlan(ctx, scene); err != nil {
			return nil, "I could not set the music: " + err.Error() + ". You can still upload a track in Edit video under Audio."
		}
		if err := s.database.UpdateScene(scene); err != nil {
			return nil, "I could not save the music just now. Ask me again in a moment."
		}
		batch = append(batch, map[string]any{"id": scene.ID, "title": scene.Title, "duration": scene.Duration})
	}
	if len(batch) == 0 {
		return nil, "That episode has no scenes yet, so there is nothing to add music to."
	}
	videos := "its scene video needs"
	if len(batch) > 1 {
		videos = fmt.Sprintf("its %d scene videos need", len(batch))
	}
	if music == "" {
		return batch, fmt.Sprintf("Music is now off for episode %d. The video model makes sound together with the picture, so %s to be made again to hear the change. The button below shows the price, and your pictures are kept.", number, videos)
	}
	return batch, fmt.Sprintf("Music for episode %d is now “%s”. The video model makes the music together with the voice, so %s to be made again; the button below shows the price and your pictures are kept. Then say “finish the video” to join them. To use your own track for free instead, open Edit video and upload it under Audio.", number, music, videos)
}
