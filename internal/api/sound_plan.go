package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/models"
)

// The sound plan says who makes each part of a story's sound. Each clip is made on its
// own, so music asked of every clip is a new piece of music in every clip and it breaks
// at each cut. A story made of more than one clip therefore has clips that make their
// voices, the scene's ambience and the sounds of the action, and no music; one music
// track (the bed) is laid under the whole film in the edit (soundtrack.go). A video made
// in a single clip may still have its music made with it.
//
// It is kept in the story's draft under "sound_plan". With none saved, the default
// below applies; the AI may decide one (ambience per scene and the music's direction),
// and what it decides is kept only when checkSoundPlan accepts it.
type soundPlan struct {
	Music    soundMusic      `json:"music"`
	Ambience []sceneAmbience `json:"ambience"`
	// ClipAudio is what each clip's own sound holds.
	ClipAudio string `json:"clip_audio"`
	// DecidedBy is "default", "ai" or "owner".
	DecidedBy string `json:"decided_by"`
}

type soundMusic struct {
	// Mode is "bed" (one track under the whole film), "none", or "clip" (the one clip
	// of a single-clip video makes its own music).
	Mode   string `json:"mode"`
	Prompt string `json:"prompt,omitempty"`
	// Source is the asset the bed plays: an upload, a library track, or one made once
	// from Prompt. Empty until there is one.
	Source string `json:"source,omitempty"`
}

type sceneAmbience struct {
	SceneID  string `json:"scene_id"`
	Ambience string `json:"ambience"`
}

const (
	musicBed    = "bed"
	musicNone   = "none"
	musicInClip = "clip"

	clipDialogueAmbience = "dialogue_and_ambience"
	clipWithMusic        = "dialogue_ambience_and_music"

	decidedDefault = "default"
	decidedAI      = "ai"
	decidedOwner   = "owner"

	// musicBedAsset is the asset type of a bed made once from the plan's direction.
	musicBedAsset = "music_bed"
)

// musicInClip says whether the clips make their own music.
func (p soundPlan) musicInClip() bool { return p.ClipAudio == clipWithMusic }

// ambienceFor is a scene's steady background sound, empty when none was chosen.
func (p soundPlan) ambienceFor(sceneID string) string {
	for _, a := range p.Ambience {
		if a.SceneID == sceneID {
			return a.Ambience
		}
	}
	return ""
}

// defaultSoundPlan is the plan with nothing decided: a story of several clips gets one
// music bed when music was asked for, and a single clip makes its own.
func defaultSoundPlan(clips int, music string) soundPlan {
	p := soundPlan{Music: soundMusic{Mode: musicNone}, Ambience: []sceneAmbience{}, ClipAudio: clipDialogueAmbience, DecidedBy: decidedDefault}
	music = strings.TrimSpace(music)
	switch {
	case music == "":
	case clips <= 1:
		p.Music, p.ClipAudio = soundMusic{Mode: musicInClip, Prompt: music}, clipWithMusic
	default:
		p.Music = soundMusic{Mode: musicBed, Prompt: music}
	}
	return p
}

// musicWords are words that would bring music back into a clip through its ambience.
var musicWords = regexp.MustCompile(`(?i)\b(music|musical|bgm|score|song|songs|melody|instrumental|soundtrack|singing|choir|chorus|beat|beats|piano|guitar|strings|drums|synth)\b`)

// checkSoundPlan says whether a plan may be used for a story made in clips clips whose
// scenes are scenes. assets says whether an asset id is an audio file of the project.
func checkSoundPlan(p soundPlan, clips int, scenes map[string]bool, audio func(string) bool) error {
	switch p.Music.Mode {
	case musicBed, musicNone, musicInClip:
	default:
		return fmt.Errorf("music mode must be bed, none or clip")
	}
	switch p.ClipAudio {
	case clipDialogueAmbience, clipWithMusic:
	default:
		return fmt.Errorf("clip audio must be %s or %s", clipDialogueAmbience, clipWithMusic)
	}
	if (p.Music.Mode == musicInClip) != (p.ClipAudio == clipWithMusic) {
		return fmt.Errorf("clips make their own music only when the music mode is clip")
	}
	if clips > 1 && p.ClipAudio != clipDialogueAmbience {
		return fmt.Errorf("the story is made in %d clips: music made in each clip restarts at every cut, so the clips get dialogue and ambience only and the music is one bed in the edit", clips)
	}
	prompt := strings.TrimSpace(p.Music.Prompt)
	if len(prompt) > 300 {
		return fmt.Errorf("the music direction must be at most 300 characters")
	}
	if p.Music.Mode != musicNone && prompt == "" && p.Music.Source == "" {
		return fmt.Errorf("music needs a direction or a track")
	}
	if p.Music.Mode == musicNone && (prompt != "" || p.Music.Source != "") {
		return fmt.Errorf("no music was chosen, so there is no direction or track to keep")
	}
	if p.Music.Source != "" && (p.Music.Mode != musicBed || audio == nil || !audio(p.Music.Source)) {
		return fmt.Errorf("the music track must be an audio file of this project, played as the bed")
	}
	seen := map[string]bool{}
	for _, a := range p.Ambience {
		text := strings.TrimSpace(a.Ambience)
		switch {
		case !scenes[a.SceneID]:
			return fmt.Errorf("ambience is for a scene that is not in this story")
		case seen[a.SceneID]:
			return fmt.Errorf("a scene has two ambiences")
		case text == "" || len(text) > 200:
			return fmt.Errorf("a scene's ambience must be 1-200 characters")
		case musicWords.MatchString(text):
			return fmt.Errorf("the ambience of a scene mentions music (%q); music is the one bed in the edit", musicWords.FindString(text))
		case strings.ContainsAny(text, "\"“”{}"):
			return fmt.Errorf("a scene's ambience must not quote speech")
		}
		seen[a.SceneID] = true
	}
	return nil
}

// storyScenes are a story's scenes in story order.
func (s *Server) storyScenes(project, scriptID string) []models.Scene {
	all, _ := s.database.ListScenes(project) // unreadable scenes leave the story empty
	out := []models.Scene{}
	for _, sc := range all {
		if sc.ScriptID != nil && *sc.ScriptID == scriptID {
			out = append(out, sc)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SceneOrder < out[j].SceneOrder })
	return out
}

// storyScript is a story of the project, or nil.
func (s *Server) storyScript(project, scriptID string) *models.Script {
	scripts, _ := s.database.ListScripts(project) // unreadable stories read as missing
	for i := range scripts {
		if scripts[i].ID == scriptID {
			return &scripts[i]
		}
	}
	return nil
}

// sceneClips is how many clips a scene is made in: its production plan's, else one.
func (s *Server) sceneClips(ctx context.Context, scene *models.Scene) int {
	if plan, err := s.sceneProductionPlan(ctx, scene); err == nil && len(plan.Units) > 0 {
		return len(plan.Units)
	}
	return 1
}

// sceneMusic is the music asked for a scene: its own direction, else the story's.
func sceneMusic(scene *models.Scene) string {
	if music := sceneProduction(scene).MusicPrompt; music != "" {
		return music
	}
	return sceneBrief(scene).Music
}

// projectAudio says whether an asset id is an audio file of the project.
func (s *Server) projectAudio(project string) func(string) bool {
	return func(id string) bool {
		a, err := s.database.GetAsset(id)
		if err != nil || a.ProjectID == nil || *a.ProjectID != project {
			return false
		}
		switch strings.ToLower(filepath.Ext(a.FilePath)) {
		case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus":
			return true
		}
		return false
	}
}

// storySound is a story's sound plan as it applies now, with the number of clips the
// story is made in. A saved plan that no longer fits (the story grew from one clip to
// several) keeps its music direction as a bed.
func (s *Server) storySound(ctx context.Context, project, scriptID string) (soundPlan, int) {
	scenes := s.storyScenes(project, scriptID)
	clips, music, ids := 0, "", map[string]bool{}
	for i := range scenes {
		clips += s.sceneClips(ctx, &scenes[i])
		ids[scenes[i].ID] = true
		if music == "" {
			music = sceneMusic(&scenes[i])
		}
	}
	plan := defaultSoundPlan(clips, music)
	script := s.storyScript(project, scriptID)
	if script == nil {
		return plan, clips
	}
	var draft struct {
		Sound *soundPlan `json:"sound_plan"`
	}
	if json.Unmarshal(script.DraftJSON, &draft) != nil || draft.Sound == nil {
		return plan, clips
	}
	saved := *draft.Sound
	if saved.Ambience == nil {
		saved.Ambience = []sceneAmbience{}
	}
	if clips > 1 && saved.Music.Mode == musicInClip {
		saved.Music.Mode, saved.ClipAudio = musicBed, clipDialogueAmbience
	}
	if checkSoundPlan(saved, clips, ids, s.projectAudio(project)) != nil {
		// Ambience of scenes since deleted, or a track since removed, is dropped.
		kept := []sceneAmbience{}
		for _, a := range saved.Ambience {
			if ids[a.SceneID] {
				kept = append(kept, a)
			}
		}
		saved.Ambience = kept
		if saved.Music.Source != "" && !s.projectAudio(project)(saved.Music.Source) {
			saved.Music.Source = ""
		}
	}
	if checkSoundPlan(saved, clips, ids, s.projectAudio(project)) != nil {
		return plan, clips
	}
	return saved, clips
}

// sceneSound is the sound plan a scene's clips are made with: its story's, or for a
// scene outside a story, the default for the clips it is made in.
func (s *Server) sceneSound(ctx context.Context, scene *models.Scene) soundPlan {
	if scene.ScriptID != nil && scene.ProjectID != nil {
		plan, _ := s.storySound(ctx, *scene.ProjectID, *scene.ScriptID)
		return plan
	}
	return defaultSoundPlan(s.sceneClips(ctx, scene), sceneMusic(scene))
}

// saveSoundPlan checks a plan for a story and keeps it.
func (s *Server) saveSoundPlan(ctx context.Context, project, scriptID string, plan soundPlan) (soundPlan, error) {
	scenes := s.storyScenes(project, scriptID)
	clips, ids := 0, map[string]bool{}
	for i := range scenes {
		clips += s.sceneClips(ctx, &scenes[i])
		ids[scenes[i].ID] = true
	}
	if plan.Ambience == nil {
		plan.Ambience = []sceneAmbience{}
	}
	plan.Music.Prompt = strings.TrimSpace(plan.Music.Prompt)
	for i := range plan.Ambience {
		plan.Ambience[i].Ambience = strings.TrimRight(strings.TrimSpace(plan.Ambience[i].Ambience), ".")
	}
	if err := checkSoundPlan(plan, clips, ids, s.projectAudio(project)); err != nil {
		return plan, err
	}
	raw, _ := json.Marshal(plan)
	return plan, s.database.SetScriptSoundPlan(scriptID, raw)
}

// askSoundPlan asks the AI for a story's sound plan. Tests replace it.
var askSoundPlan = func(s *Server, ctx context.Context, system, facts string) (soundPlan, error) {
	var out soundPlan
	err := s.agentJSON(ctx, "prompt_agent", system, facts, nil, &out)
	return out, err
}

const soundPlanRules = `You are the sound designer of a short film made of separately generated video clips. Each clip is made by a video model with its own sound (Google Gemini Omni or ByteDance Seedance), on its own, so anything asked of every clip is made new in every clip.
Decide the film's sound plan:
- music: one music bed under the whole film, added in the edit (mode "bed"), or none (mode "none"). With more than one clip never ask the clips for music: each clip would invent its own and it would break at every cut. Only a film made in a single clip may make its music in the clip (mode "clip"). The prompt is a short English direction (instrument, tempo, mood, no lyrics), for example "slow sparse piano with low strings, 70 bpm". Keep music the user chose; "none" only when they asked for none.
- ambience: for each scene, its steady background sound in a few words ("light rain on a tin roof, distant traffic"). Every clip of the scene is told the same words so the room does not change at a cut. Never mention music, instruments or singing there, and never quote speech.
- clip_audio: "dialogue_and_ambience" (clips make voices, ambience and the sounds of the action, no music); "dialogue_ambience_and_music" only for a single-clip film whose music is made in the clip.
Return JSON: {"music": {"mode": "bed"|"none"|"clip", "prompt": "..."}, "ambience": [{"scene_id": "...", "ambience": "..."}], "clip_audio": "..."}.`

// decideSoundPlan has the AI decide a story's sound plan once, when none is saved. A
// decision the checks refuse is not kept: the default plan applies instead.
func (s *Server) decideSoundPlan(ctx context.Context, project, scriptID string) soundPlan {
	current, clips := s.storySound(ctx, project, scriptID)
	if current.DecidedBy != decidedDefault {
		return current
	}
	script := s.storyScript(project, scriptID)
	if script == nil {
		return current
	}
	scenes := s.storyScenes(project, scriptID)
	type sceneFacts struct {
		ID      string `json:"scene_id"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Seconds int    `json:"seconds"`
		Speech  string `json:"speech"`
	}
	facts := map[string]any{"title": script.Title, "summary": script.Summary, "clips": clips, "music_asked_for": current.Music.Prompt}
	list := []sceneFacts{}
	for i := range scenes {
		brief := sceneBrief(&scenes[i])
		facts["tone"] = brief.Tone
		list = append(list, sceneFacts{ID: scenes[i].ID, Title: scenes[i].Title, Summary: scenes[i].Summary, Seconds: scenes[i].Duration, Speech: brief.ConversationMode})
	}
	facts["scenes"] = list
	raw, _ := json.Marshal(facts)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	plan, err := askSoundPlan(s, ctx, soundPlanRules, string(raw))
	if err == nil {
		plan.DecidedBy, plan.Music.Source = decidedAI, ""
		plan, err = s.saveSoundPlan(ctx, project, scriptID, plan)
	}
	if err != nil {
		slog.Warn("the AI's sound plan was not used; the default applies", "script", scriptID, "reason", err.Error())
		return current
	}
	return plan
}

// soundPlanView is GET /scripts/{id}/sound-plan.
type soundPlanView struct {
	Plan  soundPlan `json:"plan"`
	Clips int       `json:"clips"`
	// Line says in plain words who makes the sound.
	Line string `json:"line"`
}

func (s *Server) handleSoundPlan(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	scriptID := chi.URLParam(r, "id")
	if s.storyScript(project, scriptID) == nil {
		http.Error(w, "story is not in the active project", 404)
		return
	}
	if r.Method == http.MethodPut {
		var plan soundPlan
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&plan) != nil {
			http.Error(w, "invalid sound plan", 400)
			return
		}
		plan.DecidedBy = decidedOwner
		if _, err = s.saveSoundPlan(r.Context(), project, scriptID, plan); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
	}
	if r.Method == http.MethodPost {
		s.decideSoundPlan(r.Context(), project, scriptID)
	}
	plan, clips := s.storySound(r.Context(), project, scriptID)
	writeJSON(w, 200, soundPlanView{Plan: plan, Clips: clips, Line: s.soundLine(r.Context(), project, scriptID, plan)})
}

// soundLine says who makes a story's sound: "Sound: the clips' own voices and ambience
// (Gemini Omni); music: one bed under the whole film from your upload “x.mp3”".
func (s *Server) soundLine(ctx context.Context, project, scriptID string, plan soundPlan) string {
	service := "the video model"
	if provider, model := s.routes().MediaSelection("video"); provider != "" {
		service = videoServiceName(provider, model)
	}
	silent := true
	for _, sc := range s.storyScenes(project, scriptID) {
		silent = silent && sceneProduction(&sc).AudioMode == "silent"
	}
	voices := fmt.Sprintf("the clips' own voices, ambience and action sounds (%s)", service)
	if silent && len(s.storyScenes(project, scriptID)) > 0 {
		voices = "none from the clips (they are made silent; voices are added in the edit)"
	}
	return "Sound: " + voices + "; music: " + s.musicLine(project, plan)
}

// musicLine says where a story's music comes from.
func (s *Server) musicLine(project string, plan soundPlan) string {
	switch plan.Music.Mode {
	case musicNone:
		return "none"
	case musicInClip:
		return fmt.Sprintf("made in the single clip (“%s”)", plan.Music.Prompt)
	}
	if plan.Music.Source != "" {
		if a, err := s.database.GetAsset(plan.Music.Source); err == nil {
			return fmt.Sprintf("one bed under the whole film from %s “%s”, lowered under speech", musicOrigin(a), a.Name)
		}
	}
	return fmt.Sprintf("one bed under the whole film (“%s”), not made yet: it is made once when the scenes are joined, or upload a track in Edit video", plan.Music.Prompt)
}

// musicOrigin says what kind of track an asset is.
func musicOrigin(a *models.Asset) string {
	switch a.Type {
	case musicBedAsset:
		return "a track made once with earshot"
	case "audio_reference":
		return "your upload"
	}
	return "the library track"
}

// videoServiceName names a video model for a person: "Gemini Omni", "Seedance".
func videoServiceName(provider, model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "omni"):
		return "Gemini Omni"
	case strings.Contains(m, "seedance"):
		return "Seedance"
	case strings.Contains(m, "veo"):
		return "Veo"
	}
	if model != "" {
		return provider + " " + model
	}
	return provider
}
