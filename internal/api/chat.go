package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
	"videoflow-go/internal/presets"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CreationBrief *agents.CreativeBrief `json:"creation_brief"`
		Message       string                `json:"message"`
		History       []map[string]string   `json:"history"`
		Selected      *chatSelection        `json:"selected"`
		Attachments   []string              `json:"attachments"`
		Auto          bool                  `json:"auto"`
		Full          bool                  `json:"full"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	projectID, _ := s.activeProjectID(r.Context())
	attachments, attachErr := s.chatAttachments(projectID, body.Attachments)
	if attachErr != nil {
		http.Error(w, attachErr.Error(), 400)
		return
	}
	selectedContext, selectionErr := s.chatSelectionContext(projectID, body.Selected)
	if selectionErr != nil {
		http.Error(w, selectionErr.Error(), 409)
		return
	}
	if selectedContext == nil || body.Selected != nil && body.Selected.Kind == "final_video" {
		// A stale selection is dropped; a final video is only context, it never narrows
		// what the assistant may do.
		body.Selected = nil
	}
	scenes, _ := s.database.ListScenes(projectID)
	chars, _ := s.database.ListCharacters(projectID)
	jobs, _ := s.database.ListRenderJobs(projectID)

	outputsList := []map[string]interface{}{}
	for _, j := range jobs {
		outs, _ := s.database.ListOutputsForJob(j.ID)
		for _, o := range outs {
			outputsList = append(outputsList, map[string]interface{}{
				"id":    o.ID,
				"video": o.VideoPath,
			})
		}
	}

	sceneOptions := []map[string]interface{}{}
	for _, sc := range scenes {
		sceneOptions = append(sceneOptions, map[string]interface{}{
			"id":    sc.ID,
			"title": sc.Title, "summary": sc.Summary, "duration": sc.Duration, "character_ids": sc.CharacterIDsJSON,
		})
	}

	charOptions := []map[string]interface{}{}
	for _, c := range chars {
		charOptions = append(charOptions, map[string]interface{}{
			"id":   c.ID,
			"name": c.Name, "description": c.Description, "appearance": c.Appearance, "personality": c.Personality, "visual_rules": c.VisualRulesJSON, "voice_rules": c.VoiceRulesJSON, "reference_asset_ids": c.ReferenceAssetIDsJSON,
		})
	}

	assets, _ := s.database.ListAssets(projectID)
	assetContext := []map[string]any{}
	for i, a := range assets {
		if i >= 64 {
			break
		}
		assetContext = append(assetContext, map[string]any{"id": a.ID, "name": a.Name, "type": a.Type, "character_id": a.CharacterID, "description": a.Description, "metadata": a.MetadataJSON, "reference_usable": models.ReferenceApproved(a.MetadataJSON)})
	}
	graph, graphErr := s.database.GetGraphData(projectID)
	if graphErr != nil {
		http.Error(w, "could not load project connections", 500)
		return
	}
	style, _ := s.database.GetStyleGuide(projectID)
	availableStyles := []map[string]string{}
	if style != nil && strings.TrimSpace(style.StylePrompt) != "" {
		availableStyles = append(availableStyles, map[string]string{"id": "project-style", "name": style.Name, "description": style.StylePrompt})
	}
	for _, p := range presets.Builtins().Presets {
		availableStyles = append(availableStyles, map[string]string{"id": p.ID, "name": p.Name, "description": p.Description})
	}
	if packs, _, e := s.presetPacks(); e == nil {
		for _, pack := range packs {
			for _, p := range pack.Presets {
				availableStyles = append(availableStyles, map[string]string{"id": "community:" + pack.ID + ":" + p.ID, "name": p.Name, "description": p.Description})
			}
		}
	}
	progress, _ := s.projectProgress(r.Context(), projectID)
	failedShown, _ := s.database.FailedRendersShown(projectID)
	contextJSON, _ := json.Marshal(map[string]any{"production_progress": progress, "recent_activity": s.chatActivity(projectID, jobs), "available_visual_styles": availableStyles, "available_storytelling_methods": presets.StorytellingPresets(), "series": map[string]any{"episodes": s.seriesEpisodes(projectID)}, "video_service": s.chatVideoService(), "episode_sound": s.episodeSound(projectID), "failed_video_attempts": failedShown, "assistant_mode": chatMode(body.Auto, body.Full), "attached_media": attachments, "current_creation_brief": body.CreationBrief, "style": style, "assets": assetContext, "scenes": sceneOptions, "characters": charOptions, "outputs": outputsList, "selected_item": selectedContext, "canvas": graph, "workflow_policy": map[string]any{"image_approval_required": false, "render_audio_modes": []string{"native", "silent"}, "render_strategy": "automatic consecutive shot grouping; explicit paid-call plan", "source_clip_required_before_render": false}})
	ctx := r.Context()
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	snapshot, err := s.buildDirectorContext(projectID, binding.Selection)
	if err != nil {
		chatContextError(w)
		return
	}
	ctx = context.WithValue(ctx, directorSnapshotKey{}, snapshot)
	body.History = nil
	ctx = context.WithValue(ctx, directorUserMessageKey{}, body.Message)
	if previews, dir, e := attachmentPreviews(ctx, attachments); e == nil && len(previews) > 0 {
		defer os.RemoveAll(dir)
		ctx = context.WithValue(ctx, directorImagesKey{}, previews)
	}
	res, err := s.agents.Chat(ctx, body.Message, body.History, string(contextJSON))
	if err != nil {
		status := 500
		if errors.Is(err, errDirectorContextChanged) {
			status = 409
		}
		http.Error(w, err.Error(), status)
		return
	}

	if body.Selected != nil {
		current, e := s.chatSelectionContext(projectID, body.Selected)
		if e != nil || !bytes.Equal(current, selectedContext) {
			http.Error(w, "Selected item changed during planning; review it and try again", 409)
			return
		}
	}
	action := canonicalChatAction(res.Action)
	if res.Action == "make_clip" {
		// The one-off clip from a picture was removed; a planner that still answers
		// with it must not leave its promise standing.
		res.Reply = retiredClipReply
	}
	// retry_render redoes a finished video. When there is no finished video to redo (the
	// scene's video failed or was never made), "try again" means making the scene video.
	if action == "retry_render" && !(body.Selected != nil && body.Selected.Kind == "output") {
		finished := res.OutputID == "" && len(outputsList) == 1
		for _, output := range outputsList {
			finished = finished || output["id"] == res.OutputID
		}
		if !finished {
			action = "render_scene"
		}
	}
	// A selection is the default target, not a cage: when the user clearly asks for
	// something else (another scene, or a project-level step), follow the request.
	selectionApplies := body.Selected != nil && selectedChatActionAllowed(body.Selected.Kind, action)

	intent := map[string]interface{}{
		"action":      action,
		"reply":       res.Reply,
		"idea":        body.Message,
		"scene_count": 1,
	}

	if action == "generate_script" {
		brief := mergeCreativeBrief(body.CreationBrief, res.CreationBrief, body.Message)
		if res.NextEpisode || (body.CreationBrief != nil && strings.Contains(body.CreationBrief.Instruction, "of a series. Story so far")) {
			if cast, number := s.continueSeries(projectID, &brief, body.CreationBrief != nil); number > 0 {
				intent["cast_ids"] = cast
				intent["episode_number"] = number
			}
		}
		fillCreativeBriefDefaults(&brief, style != nil && strings.TrimSpace(style.StylePrompt) != "")
		if brief.PresetID == "project-style" {
			if style != nil {
				brief.StyleFingerprint = creativeStyleFingerprint(style)
			}
		} else if brief.PresetID != "" {
			if p, e := s.findPreset(brief.PresetID); e == nil {
				brief.StyleFingerprint = creativeStyleFingerprint(p)
			}
		}
		intent["creation_brief"] = brief
		intent["idea"] = brief.Idea
		intent["scene_count"] = brief.SceneCount
		intent["reply"] = creativeBriefQuestion(brief)
		// The fixed question is English. When the user writes another language, keep the
		// planner's own question, which it writes in the user's language.
		if lang := strings.ToLower(strings.TrimSpace(res.UserLanguage)); lang != "" && !strings.HasPrefix(lang, "en") && strings.TrimSpace(res.Reply) != "" {
			intent["reply"] = res.Reply
		} else if number, ok := intent["episode_number"].(int); ok {
			intent["reply"] = fmt.Sprintf("Episode %d: %s\n\n%s", number, brief.Idea, intent["reply"])
		}
	}
	if v := strings.TrimSpace(res.Instruction); v != "" && len(v) <= 4000 && action != "generate_script" && action != "unknown" {
		// The planner's own worked-out direction fills the button, not the user's raw words.
		intent["idea"] = v
	}
	if action == "update_character" {
		changes := map[string]string{}
		if c := res.CharacterChanges; c != nil {
			for key, value := range map[string]string{"appearance": c.Appearance, "description": c.Description, "personality": c.Personality} {
				if v := strings.TrimSpace(value); v != "" && len(v) <= 4000 {
					changes[key] = v
				}
			}
		}
		intent["character_changes"] = changes
	}
	episodes := s.seriesEpisodes(projectID)
	if action == "finish_video" {
		if script := chatEpisodeScript(scenes, episodes, res); script == "" {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = "There is no story to finish yet. Tell me your idea and I'll plan one."
		} else {
			intent["script_id"] = script
		}
	}
	if action == "render_scenes" {
		batch := chatSceneBatch(scenes, episodes, progress, res)
		ids := []string{}
		for _, scene := range batch {
			ids = append(ids, scene["id"].(string))
		}
		if len(batch) == 0 {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = "Every scene there already has a video or is being made. Say “finish the video” and I'll join them and export it."
		} else {
			intent["scene_ids"], intent["scenes"] = ids, batch
		}
	}
	if action == "check_video" || action == "stop_waiting" {
		if waiting := chatWaitingScene(progress, scenes, res, body.Selected); waiting == nil {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = "No scene is waiting on an answer from the video service right now, so there is nothing to check."
		} else {
			intent["job_id"], intent["scene_id"], intent["scene_title"], intent["service"], intent["waiting_reason"] = waiting.Video.JobID, waiting.SceneID, waiting.Title, waiting.Video.Service, waiting.Video.Reason
		}
	}
	if action == "set_music" {
		// Saving the direction is free and done at once; making the videos again is the
		// paid step, offered as one button with its price.
		if batch, reply := s.chatSetMusic(r.Context(), scenes, s.seriesEpisodes(projectID), res); len(batch) == 0 {
			action = "unknown"
			intent["action"], intent["reply"] = action, reply
		} else {
			ids := []string{}
			for _, scene := range batch {
				ids = append(ids, scene["id"].(string))
			}
			action = "render_scenes"
			intent["action"], intent["reply"], intent["scene_ids"], intent["scenes"] = action, reply, ids, batch
		}
	}
	if action == "clear_failed_videos" {
		// Free and nothing is deleted, so the assistant does it straight away.
		switch n, e := s.database.ClearFailedRenders(projectID); {
		case e != nil:
			action = "unknown"
			intent["action"], intent["reply"] = action, "I could not clear the failed videos just now. Ask me again in a moment."
		case n == 0:
			action = "unknown"
			intent["action"], intent["reply"] = action, "There are no failed video attempts left to clear."
		default:
			intent["cleared"], intent["reply"] = n, clearedFailedReply(n)
		}
	}
	if action == "switch_video_service" {
		if choice, why := s.chatVideoSwitch(res.SwitchTo); choice == nil {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = why
		} else {
			intent["video_switch"] = choice
		}
	}
	if action == "open_editor" {
		switch res.EditorPanel {
		case "voice", "music", "captions", "export":
			intent["editor_panel"] = res.EditorPanel
		default:
			intent["editor_panel"] = "edit"
		}
	}
	switch res.AssetType {
	case "prop", "location", "background", "other":
		intent["asset_type"] = res.AssetType
	}
	if action != "generate_script" && res.SceneCount >= 1 && res.SceneCount <= 20 {
		intent["scene_count"] = res.SceneCount
	}
	for _, scene := range scenes {
		if scene.ID == res.SceneID || (res.SceneID == "" && len(scenes) == 1) {
			intent["scene_id"] = scene.ID
		}
	}
	if _, ok := intent["scene_id"]; !ok && sceneScopedAction(action) {
		// Agents sometimes name the scene instead of its ID; fall back to the title, then
		// to the scene the progress check already recommends for this same action.
		for _, scene := range scenes {
			if res.SceneID != "" && strings.EqualFold(strings.TrimSpace(res.SceneID), strings.TrimSpace(scene.Title)) {
				intent["scene_id"] = scene.ID
			}
		}
		if _, ok := intent["scene_id"]; !ok && progress.Next != nil && progress.Next.Next.Action == action {
			intent["scene_id"] = progress.Next.SceneID
		}
	}
	for _, character := range chars {
		if character.ID == res.CharacterID || (res.CharacterID == "" && len(chars) == 1) {
			intent["character_id"] = character.ID
		}
	}
	for _, output := range outputsList {
		if output["id"] == res.OutputID || (res.OutputID == "" && len(outputsList) == 1) {
			intent["output_id"] = output["id"]
		}
	}

	if body.Selected != nil && !selectionApplies {
		// Outside the selection only a clearly named target or a project-level step may run.
		named := action == "generate_script" || action == "generate_image" || action == "style_ingest"
		for _, scene := range scenes {
			if sceneScopedAction(action) && res.SceneID != "" && (scene.ID == res.SceneID || strings.EqualFold(strings.TrimSpace(res.SceneID), strings.TrimSpace(scene.Title))) {
				named = true
			}
		}
		for _, character := range chars {
			if (action == "generate_character_reference" || action == "update_character") && res.CharacterID == character.ID {
				named = true
			}
		}
		if !named {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = "I'm not sure which item you mean. Tell me the scene or character by name, or clear the selection above the message box to work on the whole project. Nothing was changed."
		}
	}
	namedOtherScene := false
	for _, scene := range scenes {
		if body.Selected != nil && body.Selected.Kind == "scene" && scene.ID != body.Selected.ID && (scene.ID == res.SceneID || (res.SceneID != "" && strings.EqualFold(strings.TrimSpace(res.SceneID), strings.TrimSpace(scene.Title)))) {
			namedOtherScene = true
		}
	}
	if selectionApplies && !namedOtherScene {
		switch body.Selected.Kind {
		case "character":
			if _, named := intent["character_id"]; !named || res.CharacterID == "" || action == "generate_character_reference" || action == "update_character" {
				intent["character_id"] = body.Selected.ID
			}
		case "scene":
			if action == "refine_scene" || action == "render_scene" || action == "audit_storyboard" || action == "storyboard" || action == "generate_shots" || action == "delete_scene" {
				intent["scene_id"] = body.Selected.ID
			}
		case "shot":
			if action == "refine_shot" || action == "render_shot" {
				shot, _ := s.database.GetShot(body.Selected.ID)
				intent["shot_id"] = shot.ID
				intent["scene_id"] = shot.SceneID
			}
		case "output":
			if action == "retry_render" {
				intent["output_id"] = body.Selected.ID
			}
		}
	}
	options := map[string]interface{}{
		"scenes":     sceneOptions,
		"characters": charOptions,
		"outputs":    outputsList,
	}

	state, err := s.projectState(projectID)
	if err != nil {
		http.Error(w, "failed to load project state", 500)
		return
	}

	response := map[string]interface{}{
		"intent":   intent,
		"options":  options,
		"state":    state,
		"warnings": []string{},
	}

	currentSelectionRaw, _ := s.database.GetDirectorSelection(binding.SessionID)
	var currentSelection directorSelection
	_ = json.Unmarshal(currentSelectionRaw, &currentSelection)
	current, err := s.buildDirectorContext(projectID, currentSelection)
	if err != nil || current.Revision != snapshot.Revision {
		http.Error(w, errDirectorContextChanged.Error(), 409)
		return
	}
	selection, _ := json.Marshal(binding.Selection)
	id, err := s.database.CreateDirectorProposal(binding.SessionID, projectID, snapshot.Revision, selection)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	intent["director_proposal"] = id
	response["session_id"] = binding.SessionID
	response["context_revision"] = snapshot.Revision
	response["warnings"] = snapshot.Warnings
	writeJSON(w, http.StatusOK, response)
}

// Keep model output on the action-card contract, including legacy agent replies.
func canonicalChatAction(action string) string {
	switch action {
	case "create_script", "add_scene":
		return "generate_script"
	case "open_editor", "finish_video", "render_scenes", "check_video", "stop_waiting", "switch_video_service", "clear_failed_videos", "set_music", "update_character", "generate_image", "generate_character_reference", "generate_script", "generate_scenes", "generate_shots", "audit_storyboard", "storyboard", "render_scene", "render_shot", "retry_render", "generate_assets", "refine_scene", "refine_shot", "delete_scene", "style_ingest", "plan_assets":
		return action
	default:
		return "unknown"
	}
}

func sceneScopedAction(action string) bool {
	switch action {
	case "storyboard", "audit_storyboard", "render_scene", "generate_shots", "refine_scene", "generate_assets", "plan_assets":
		return true
	}
	return false
}

// retiredClipReply is what the user is told when a one-off clip from a picture is asked for.
const retiredClipReply = "I can't make a one-off clip straight from a picture any more. Clips are made by rendering a scene: tell me what should happen and I'll write a short scene for it. Your picture is saved in the Library; connect it to the scene's character or place and the clip is made from it."

func chatMode(auto, full bool) string {
	if full {
		return "full auto: the user wants the finished video with as few clicks as possible. For a new story they confirm the plan once; the app then makes the pictures, renders every scene, joins them with captions and exports the final video by itself, using image and video credits. Say that confirming is the only click. Free planning steps and check_video start by themselves, and finish_video and render_scenes start immediately. Decide every detail yourself and never ask the user to press per-scene buttons."
	}
	if auto {
		return "auto: free steps (refine_scene, refine_shot, generate_shots, plan_assets, check_video) start by themselves right after your reply, so write a complete instruction and say what you are doing now (\"I'm rewriting the scene so that…\"), not \"press the button\". Steps that use credits or delete things still wait for the user's click; say so. Decide details yourself and do not ask for confirmation of free steps."
	}
	return "manual: every step waits for the user to press its button."
}
