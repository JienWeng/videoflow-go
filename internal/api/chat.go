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
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/presets"
)

// chatRequest is one message sent to the assistant.
type chatRequest struct {
	CreationBrief *agents.CreativeBrief `json:"creation_brief"`
	Message       string                `json:"message"`
	History       []map[string]string   `json:"history"`
	Selected      *chatSelection        `json:"selected"`
	Attachments   []string              `json:"attachments"`
	Auto          bool                  `json:"auto"`
	Full          bool                  `json:"full"`
	// Answer is the quick reply the user picked, kept in the decision log.
	Answer *chatAnswer `json:"answer"`
}

// chatTurn is what one chat message is planned against: the request and the project
// as it stood when the message arrived.
type chatTurn struct {
	body            chatRequest
	projectID       string
	attachments     []chatAttachment
	selectedContext json.RawMessage
	scenes          []models.Scene
	chars           []models.Character
	jobs            []models.RenderJob
	outputs         []map[string]interface{}
	sceneOptions    []map[string]interface{}
	charOptions     []map[string]interface{}
	style           *models.StyleGuide
	progress        projectProgress
	// stages is the project's stage map as the turn began (see stages.go).
	stages stageMap
	// speechFit is, for each scene still to be made whose speech is tight or too long
	// for its clips, the speech budget's verdict (speech_budget.go).
	speechFit []map[string]string
	// videoService is the video service as the planner saw it (chatVideoService).
	videoService map[string]any
	// decisions is the project's decision log as the planner saw it.
	decisions []db.ProjectDecision
	// selfChecks are the pictures and clips whose self-check found problems the user
	// has not settled; a flagged clip lets Full auto ask about re-rendering it.
	selfChecks []selfCheckFlag
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	response, failure := s.runChatTurn(r.Context(), body, nil)
	if failure != nil {
		failure.write(w)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// The phases of a chat turn, in order. POST /agui shows each as an AG-UI step.
const (
	chatStepReadProject = "read_project"
	chatStepAskSession  = "ask_ai_session"
	chatStepCheckAnswer = "check_answer"
)

// chatTurnError is why a chat turn stopped, with the HTTP status /chat answers with.
// Code is set for the failures the chat panel acts on (chat_context_invalid).
type chatTurnError struct {
	status  int
	code    string
	message string
}

func (e *chatTurnError) Error() string { return e.message }

// write answers /chat with the failure, as it always has: JSON when it has a code,
// plain text otherwise.
func (e *chatTurnError) write(w http.ResponseWriter) {
	if e.code != "" {
		writeJSON(w, e.status, map[string]string{"code": e.code, "error": e.message})
		return
	}
	http.Error(w, e.message, e.status)
}

func turnFailure(status int, message string) *chatTurnError {
	return &chatTurnError{status: status, message: message}
}

// runChatTurn plans one chat message and returns the /chat response: the action card,
// the pickable options and the project state. POST /chat and POST /agui both answer
// through it. onStep, when set, hears each phase start (started) and finish.
func (s *Server) runChatTurn(ctx context.Context, body chatRequest, onStep func(name string, started bool)) (map[string]interface{}, *chatTurnError) {
	open := ""
	step := func(name string) {
		if onStep == nil {
			return
		}
		if open != "" {
			onStep(open, false)
		}
		if open = name; name != "" {
			onStep(name, true)
		}
	}
	defer step("")

	step(chatStepReadProject)
	projectID, _ := s.activeProjectID(ctx)
	turn, status, err := s.newChatTurn(projectID, body)
	if err != nil {
		return nil, turnFailure(status, err.Error())
	}
	// A quick reply settles its question before planning, so this turn already sees it.
	s.recordAnswer(projectID, body.Answer)
	contextJSON, err := s.chatPlannerContext(ctx, turn)
	if err != nil {
		return nil, turnFailure(500, "could not load project connections")
	}
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	snapshot, err := s.buildDirectorContext(projectID, binding.Selection)
	if err != nil {
		return nil, &chatTurnError{status: 409, code: chatContextInvalid, message: chatContextInvalidMessage}
	}
	turn.body.History = nil
	planCtx, previewDir := chatDirectorContext(ctx, snapshot, turn.body.Message, turn.attachments)
	if previewDir != "" {
		defer os.RemoveAll(previewDir)
	}

	step(chatStepAskSession)
	res, err := s.agents.Chat(planCtx, turn.body.Message, turn.body.History, string(contextJSON))
	if err != nil {
		status := 500
		if errors.Is(err, errDirectorContextChanged) {
			status = 409
		}
		return nil, turnFailure(status, err.Error())
	}

	step(chatStepCheckAnswer)
	if turn.body.Selected != nil {
		current, e := s.chatSelectionContext(projectID, turn.body.Selected)
		if e != nil || !bytes.Equal(current, turn.selectedContext) {
			return nil, turnFailure(409, "Selected item changed during planning; review it and try again")
		}
	}
	intent := s.chatIntent(ctx, turn, res)
	intent["remembered"] = s.rememberDecisions(projectID, res.Remember)
	return s.chatResponse(turn, intent, binding, snapshot)
}

// newChatTurn checks the message's attachments and selection and loads the project's
// scenes, characters and finished videos. A failure comes with its HTTP status.
func (s *Server) newChatTurn(projectID string, body chatRequest) (*chatTurn, int, error) {
	attachments, attachErr := s.chatAttachments(projectID, body.Attachments)
	if attachErr != nil {
		return nil, 400, attachErr
	}
	selectedContext, selectionErr := s.chatSelectionContext(projectID, body.Selected)
	if selectionErr != nil {
		return nil, 409, selectionErr
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
	return &chatTurn{
		body: body, projectID: projectID, attachments: attachments, selectedContext: selectedContext,
		scenes: scenes, chars: chars, jobs: jobs, outputs: outputsList, sceneOptions: sceneOptions, charOptions: charOptions,
	}, 0, nil
}

// chatPlannerContext builds the project context the planner reads, as JSON. It also
// records the project's style and production progress on the turn.
func (s *Server) chatPlannerContext(ctx context.Context, turn *chatTurn) ([]byte, error) {
	projectID, body := turn.projectID, turn.body
	assetContext := s.chatAssetContext(projectID)
	graph, graphErr := s.database.GetGraphData(projectID)
	if graphErr != nil {
		return nil, graphErr
	}
	style, _ := s.database.GetStyleGuide(projectID)
	availableStyles := s.chatAvailableStyles(style)
	progress, _ := s.projectProgress(ctx, projectID)
	failedShown, _ := s.database.FailedRendersShown(projectID)
	turn.style, turn.progress = style, progress
	turn.decisions, _ = s.database.ListDecisions(projectID) // an unreadable log reads as empty; the assistant may then ask again
	if in, err := s.stageInputs(ctx, projectID, progress); err == nil {
		turn.stages = computeStages(in)
		turn.speechFit = chatSpeechFit(in.Scenes)
		turn.selfChecks = append(s.flaggedPictures(projectID, turn.chars, turn.scenes), markSpeechRetried(flaggedClips(in.Clips), s.speechRetriedScenes(turn.decisions))...)
	}
	turn.videoService = s.chatVideoService()
	contextJSON, _ := json.Marshal(map[string]any{"stages": chatStages(turn.stages), "speech_fit": turn.speechFit, "self_check": chatSelfCheck(turn.selfChecks), "decided_so_far": chatDecidedSoFar(turn.decisions), "production_progress": chatProgress(progress), "recent_activity": s.chatActivity(projectID, turn.jobs), "available_visual_styles": availableStyles, "available_storytelling_methods": presets.StorytellingPresets(), "series": map[string]any{"episodes": s.seriesEpisodes(projectID)}, "video_service": turn.videoService, "episode_sound": s.episodeSound(ctx, projectID), "failed_video_attempts": failedShown, "assistant_mode": chatMode(body.Auto, body.Full), "attached_media": turn.attachments, "current_creation_brief": body.CreationBrief, "style": style, "assets": assetContext, "scenes": turn.sceneOptions, "characters": turn.charOptions, "outputs": turn.outputs, "selected_item": turn.selectedContext, "canvas": graph, "workflow_policy": map[string]any{"image_approval_required": false, "render_audio_modes": []string{"native", "silent"}, "render_strategy": "automatic consecutive shot grouping; explicit paid-call plan", "source_clip_required_before_render": false}})
	return contextJSON, nil
}

// chatProgress is the part of /production/progress the planner acts on: where each
// scene and its video stand, with the ids its actions need. The stage map carries the
// project-wide summary and next step, so the long per-scene help texts are left out.
func chatProgress(progress projectProgress) map[string]any {
	scenes := []map[string]any{}
	for _, p := range progress.Scenes {
		scene := map[string]any{"scene_id": p.SceneID, "title": p.Title, "order": p.Order, "stage": p.Stage, "shots": p.Shots, "video": p.Video, "next_action": p.Next.Action}
		if p.ReferenceMode {
			scene["reference_mode"] = true
		}
		scenes = append(scenes, scene)
	}
	out := map[string]any{"scenes": scenes, "total": progress.Total, "video_ready": progress.VideoReady, "attention": progress.Attention}
	if progress.Next != nil {
		out["next_scene_id"] = progress.Next.SceneID
	}
	return out
}

// chatAssetContext lists up to 64 of the project's library items for the planner.
func (s *Server) chatAssetContext(projectID string) []map[string]any {
	assets, _ := s.database.ListAssets(projectID)
	assetContext := []map[string]any{}
	for i, a := range assets {
		if i >= 64 {
			break
		}
		assetContext = append(assetContext, map[string]any{"id": a.ID, "name": a.Name, "type": a.Type, "character_id": a.CharacterID, "description": a.Description, "metadata": a.MetadataJSON, "reference_usable": models.ReferenceApproved(a.MetadataJSON)})
	}
	return assetContext
}

// chatAvailableStyles lists the visual styles the planner may pick: the project's own
// style first, then the built-in presets, then installed community packs.
func (s *Server) chatAvailableStyles(style *models.StyleGuide) []map[string]string {
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
	return availableStyles
}

// chatDirectorContext adds the director snapshot, the user's message and any attached
// picture previews to the planner's context. When previews were made, it returns the
// folder holding them, for the caller to remove once the reply is sent.
func chatDirectorContext(ctx context.Context, snapshot *directorContext, message string, attachments []chatAttachment) (context.Context, string) {
	ctx = context.WithValue(ctx, directorSnapshotKey{}, snapshot)
	ctx = context.WithValue(ctx, directorUserMessageKey{}, message)
	if previews, dir, e := attachmentPreviews(ctx, attachments); e == nil && len(previews) > 0 {
		return context.WithValue(ctx, directorImagesKey{}, previews), dir
	}
	return ctx, ""
}

// chatIntent turns the planner's answer into the action card: it pins the action to
// the card contract, fills in its targets and keeps it within the user's selection.
func (s *Server) chatIntent(ctx context.Context, turn *chatTurn, res *agents.ChatIntentResponse) map[string]interface{} {
	body := turn.body
	action := chatPlannedAction(turn, res)
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
		s.applyChatCreativeBrief(turn, res, intent)
	}
	if v := strings.TrimSpace(res.Instruction); v != "" && len(v) <= 4000 && action != "generate_script" && action != "unknown" {
		// The planner's own worked-out direction fills the button, not the user's raw words.
		intent["idea"] = v
	}
	if action == "update_character" {
		intent["character_changes"] = chatCharacterChanges(res)
	}
	action = s.resolveChatVideoActions(ctx, turn, res, action, intent)
	action = s.resolveChatServiceActions(turn, res, action, intent)
	action = s.resolveChatCharacterSheets(turn, res, action, intent)
	chatIntentTargets(turn, res, action, intent)
	action = resolveChatSpeechRepair(turn, res, action, intent)
	action = s.resolveChatAIEdit(turn, res, action, intent)
	action = chatGuardSelection(turn, res, action, selectionApplies, intent)
	s.pinChatSelection(turn, res, action, selectionApplies, intent)
	intent["decision"] = chatTurnDecision(turn, res, action)
	return intent
}

// chatTurnDecision validates the planner's ask-or-decide choice for the action the
// turn ends with, and applies Full auto's rule on questions.
func chatTurnDecision(turn *chatTurn, res *agents.ChatIntentResponse, action string) chatDecision {
	decision := normalizeChatDecision(res.Decision, action)
	decision = applySettledAsk(decision, action, turn.body.Message, turn.decisions)
	if turn.body.Full {
		decision = applyFullAutoAsk(decision, action, chatInputMissing(turn), turn.clipFlagged())
	}
	return decision
}

// plannerAsks says whether the turn's reply, with action, would wait for the user's
// answer. A step the server does at once while planning must then wait too.
func plannerAsks(turn *chatTurn, res *agents.ChatIntentResponse, action string) bool {
	return chatTurnDecision(turn, res, action).Mode == decisionAsk
}

// chatPlannedAction is the planner's action on the card contract. A retired one-off clip
// replaces the planner's reply, and "try again" without a finished video makes the scene.
func chatPlannedAction(turn *chatTurn, res *agents.ChatIntentResponse) string {
	action := canonicalChatAction(res.Action)
	if res.Action == "make_clip" {
		// The one-off clip from a picture was removed; a planner that still answers
		// with it must not leave its promise standing.
		res.Reply = retiredClipReply
	}
	// retry_render redoes a finished video. When there is no finished video to redo (the
	// scene's video failed or was never made), "try again" means making the scene video.
	if action == "retry_render" && !(turn.body.Selected != nil && turn.body.Selected.Kind == "output") {
		finished := res.OutputID == "" && len(turn.outputs) == 1
		for _, output := range turn.outputs {
			finished = finished || output["id"] == res.OutputID
		}
		if !finished {
			action = "render_scene"
		}
	}
	return action
}

// applyChatCreativeBrief fills a new story's card: the merged creative brief, a series'
// next episode, the chosen style and the question the user answers to confirm.
func (s *Server) applyChatCreativeBrief(turn *chatTurn, res *agents.ChatIntentResponse, intent map[string]interface{}) {
	body, style := turn.body, turn.style
	brief := mergeCreativeBrief(body.CreationBrief, res.CreationBrief, body.Message)
	if res.NextEpisode || (body.CreationBrief != nil && strings.Contains(body.CreationBrief.Instruction, "of a series. Story so far")) {
		if cast, number := s.continueSeries(turn.projectID, &brief, body.CreationBrief != nil); number > 0 {
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

// chatCharacterChanges keeps the planner's non-empty, bounded character edits.
func chatCharacterChanges(res *agents.ChatIntentResponse) map[string]string {
	changes := map[string]string{}
	if c := res.CharacterChanges; c != nil {
		for key, value := range map[string]string{"appearance": c.Appearance, "description": c.Description, "personality": c.Personality} {
			if v := strings.TrimSpace(value); v != "" && len(v) <= 4000 {
				changes[key] = v
			}
		}
	}
	return changes
}

// resolveChatVideoActions fills the targets of the actions that make or check scene
// videos. An action with nothing to act on becomes "unknown" with a reply saying why.
func (s *Server) resolveChatVideoActions(ctx context.Context, turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]interface{}) string {
	scenes, progress := turn.scenes, turn.progress
	episodes := s.seriesEpisodes(turn.projectID)
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
		if waiting := chatWaitingScene(progress, scenes, res, turn.body.Selected); waiting == nil {
			action = "unknown"
			intent["action"] = action
			intent["reply"] = "No scene is waiting on an answer from the video service right now, so there is nothing to check."
		} else {
			intent["job_id"], intent["scene_id"], intent["scene_title"], intent["service"], intent["waiting_reason"] = waiting.Video.JobID, waiting.SceneID, waiting.Title, waiting.Video.Service, waiting.Video.Reason
		}
	}
	if action == "set_music" && plannerAsks(turn, res, action) {
		// Saving the music is done at once, so a reply that asks first saves nothing.
		action = "unknown"
		intent["action"] = action
	}
	if action == "set_music" {
		// Saving the direction is free and done at once; making the videos again is the
		// paid step, offered as one button with its price.
		if batch, reply := s.chatSetMusic(ctx, scenes, s.seriesEpisodes(turn.projectID), res); len(batch) == 0 {
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
	return action
}

// resolveChatServiceActions handles clearing failed videos (done at once, it is free),
// switching the video service and which editor panel to open.
func (s *Server) resolveChatServiceActions(turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]interface{}) string {
	if action == "clear_failed_videos" && plannerAsks(turn, res, action) {
		// Clearing is done at once, so a reply that asks first clears nothing.
		action = "unknown"
		intent["action"] = action
	}
	if action == "clear_failed_videos" {
		// Free and nothing is deleted, so the assistant does it straight away.
		switch n, e := s.database.ClearFailedRenders(turn.projectID); {
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
	return action
}

// resolveChatCharacterSheets lists the characters generate_character_references draws
// and its picture calls, for the button to show before anything is spent: the ones the
// planner named (by id or name), or else every character without a usable picture.
func (s *Server) resolveChatCharacterSheets(turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]interface{}) string {
	if action != "generate_character_references" {
		return action
	}
	named := []string{}
	for _, want := range res.CharacterIDs {
		for _, c := range turn.chars {
			if c.ID == want || strings.EqualFold(strings.TrimSpace(want), strings.TrimSpace(c.Name)) {
				named = append(named, c.ID)
			}
		}
	}
	chars, err := s.sheetCharacters(turn.projectID, named)
	if err != nil || len(chars) == 0 {
		action = "unknown"
		intent["action"] = action
		intent["reply"] = "Every character already has a reference picture, so there is nothing to make. To redo one, say which character."
		return action
	}
	ids, list := []string{}, []map[string]string{}
	for _, c := range chars {
		ids = append(ids, c.ID)
		list = append(list, map[string]string{"id": c.ID, "name": c.Name})
	}
	intent["character_ids"], intent["characters"], intent["image_calls"] = ids, list, len(chars)*characterSheetImageCalls
	return action
}

// chatIntentTargets fills the asset type, scene count and the scene, character and
// finished video the planner named, matching IDs first and scene titles as a fallback.
func chatIntentTargets(turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]interface{}) {
	scenes, chars, progress := turn.scenes, turn.chars, turn.progress
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
	for _, output := range turn.outputs {
		if output["id"] == res.OutputID || (res.OutputID == "" && len(turn.outputs) == 1) {
			intent["output_id"] = output["id"]
		}
	}
}

// chatGuardSelection keeps an action outside the user's selection from running unless
// it clearly names its own target or is a project-level step.
func chatGuardSelection(turn *chatTurn, res *agents.ChatIntentResponse, action string, selectionApplies bool, intent map[string]interface{}) string {
	if turn.body.Selected == nil || selectionApplies {
		return action
	}
	// Outside the selection only a clearly named target or a project-level step may run.
	named := action == "generate_script" || action == "generate_image" || action == "style_ingest"
	for _, scene := range turn.scenes {
		if sceneScopedAction(action) && res.SceneID != "" && (scene.ID == res.SceneID || strings.EqualFold(strings.TrimSpace(res.SceneID), strings.TrimSpace(scene.Title))) {
			named = true
		}
	}
	for _, character := range turn.chars {
		if (action == "generate_character_reference" || action == "update_character") && res.CharacterID == character.ID {
			named = true
		}
	}
	if !named {
		action = "unknown"
		intent["action"] = action
		intent["reply"] = "I'm not sure which item you mean. Tell me the scene or character by name, or clear the selection above the message box to work on the whole project. Nothing was changed."
	}
	return action
}

// pinChatSelection points the action at the selected item, unless the planner named a
// different scene.
func (s *Server) pinChatSelection(turn *chatTurn, res *agents.ChatIntentResponse, action string, selectionApplies bool, intent map[string]interface{}) {
	selected := turn.body.Selected
	namedOtherScene := false
	for _, scene := range turn.scenes {
		if selected != nil && selected.Kind == "scene" && scene.ID != selected.ID && (scene.ID == res.SceneID || (res.SceneID != "" && strings.EqualFold(strings.TrimSpace(res.SceneID), strings.TrimSpace(scene.Title)))) {
			namedOtherScene = true
		}
	}
	if !selectionApplies || namedOtherScene {
		return
	}
	switch selected.Kind {
	case "character":
		if _, named := intent["character_id"]; !named || res.CharacterID == "" || action == "generate_character_reference" || action == "update_character" {
			intent["character_id"] = selected.ID
		}
	case "scene":
		if action == "refine_scene" || action == "render_scene" || action == "audit_storyboard" || action == "storyboard" || action == "generate_shots" || action == "delete_scene" {
			intent["scene_id"] = selected.ID
		}
	case "shot":
		if action == "refine_shot" || action == "render_shot" {
			shot, _ := s.database.GetShot(selected.ID)
			intent["shot_id"] = shot.ID
			intent["scene_id"] = shot.SceneID
		}
	case "output":
		if action == "retry_render" {
			intent["output_id"] = selected.ID
		}
	}
}

// chatResponse is the action card, the pickable options and the project state, after
// checking the director context did not change while planning and recording the
// proposal.
func (s *Server) chatResponse(turn *chatTurn, intent map[string]interface{}, binding directorBinding, snapshot *directorContext) (map[string]interface{}, *chatTurnError) {
	projectID := turn.projectID
	options := map[string]interface{}{
		"scenes":     turn.sceneOptions,
		"characters": turn.charOptions,
		"outputs":    turn.outputs,
	}

	state, err := s.projectState(projectID)
	if err != nil {
		return nil, turnFailure(500, "failed to load project state")
	}

	response := map[string]interface{}{
		"intent":   intent,
		"options":  options,
		"state":    state,
		"warnings": []string{},
	}

	currentSelectionRaw, _ := s.database.GetDirectorSelection(binding.SessionID)
	var currentSelection directorSelection
	_ = json.Unmarshal(currentSelectionRaw, &currentSelection) // a missing or unreadable selection reads as none; the revision check rejects it
	current, err := s.buildDirectorContext(projectID, currentSelection)
	if err != nil || current.Revision != snapshot.Revision {
		return nil, turnFailure(409, errDirectorContextChanged.Error())
	}
	selection, _ := json.Marshal(binding.Selection)
	id, err := s.database.CreateDirectorProposal(binding.SessionID, projectID, snapshot.Revision, selection)
	if err != nil {
		return nil, turnFailure(500, err.Error())
	}
	intent["director_proposal"] = id
	response["session_id"] = binding.SessionID
	response["context_revision"] = snapshot.Revision
	response["warnings"] = snapshot.Warnings
	return response, nil
}

// Keep model output on the action-card contract, including legacy agent replies.
func canonicalChatAction(action string) string {
	switch action {
	case "create_script", "add_scene":
		return "generate_script"
	case "open_editor", "finish_video", "render_scenes", "check_video", "stop_waiting", "switch_video_service", "clear_failed_videos", "set_music", "update_character", "generate_image", "generate_character_reference", "generate_character_references", "generate_script", "generate_scenes", "generate_shots", "audit_storyboard", "storyboard", "render_scene", "render_shot", "retry_render", "generate_assets", "refine_scene", "refine_shot", "delete_scene", "style_ingest", "plan_assets", "fit_script", speechRepairAction, aiEditAction:
		return action
	default:
		return "unknown"
	}
}

func sceneScopedAction(action string) bool {
	switch action {
	case "storyboard", "audit_storyboard", "render_scene", "generate_shots", "refine_scene", "generate_assets", "plan_assets", "fit_script":
		return true
	}
	return false
}

// retiredClipReply is what the user is told when a one-off clip from a picture is asked for.
const retiredClipReply = "I can't make a one-off clip straight from a picture any more. Clips are made by rendering a scene: tell me what should happen and I'll write a short scene for it. Your picture is saved in the Library; connect it to the scene's character or place and the clip is made from it."

func chatMode(auto, full bool) string {
	if full {
		return "full auto: the user wants the finished video with as few clicks as possible. For a new story they confirm the plan once; the app then makes the pictures, renders every scene, joins them with captions and exports the final video by itself, using image and video credits. Say that confirming is the only click. Free planning steps and check_video start by themselves, and finish_video, render_scenes, generate_character_references, generate_character_reference, generate_scenes, storyboard, audit_storyboard, generate_assets, render_scene, render_shot, retry_render, generate_image and fit_script start by themselves as soon as they are ready (fit_script also saves the fitted lines); repair_clip_speech also starts by itself, but only once per clip: it is the one automatic re-render a clip whose speech the check flagged gets, and after it the clip is asked about; finishing the video also runs ai_edit once and keeps its proposal only when every check passes, and an ai_edit card starts by itself; only delete_scene and edits to a character's details wait for the user; finish_video and render_scenes first make the missing reference pictures of the scenes' characters, all at once, so never offer per-character picture buttons first. Decide every detail yourself and never ask the user to press per-scene buttons. The only questions allowed are the one plan confirmation, a destructive step, a required input that is missing, and about a flagged clip: for its speech only once its one automatic retry is done (say you already shortened the line and re-rendered it once) or when it spoke the wrong language or nothing; any other question is overruled and the step goes ahead."
	}
	if auto {
		return "auto: free steps (refine_scene, refine_shot, generate_shots, plan_assets, check_video) start by themselves right after your reply, so write a complete instruction and say what you are doing now (\"I'm rewriting the scene so that…\"), not \"press the button\". Steps that use credits or delete things still wait for the user's click, and so does fit_script (the user sees the script before and after and chooses), and so does repair_clip_speech (the user sees the shorter lines and the price of the new clip, then chooses), and so does ai_edit (a proposal the user accepts or discards); say so. Decide details yourself and do not ask for confirmation of free steps."
	}
	return "manual: every step waits for the user to press its button."
}

// chatSpeechFit tells the assistant which scenes still to be made cannot say their lines
// in time, or only just, so it can offer fit_script before their video is made.
func chatSpeechFit(scenes []stageScene) []map[string]string {
	out := []map[string]string{}
	for _, sc := range scenes {
		if sc.Speech == "tight" || sc.Speech == "too_long" {
			out = append(out, map[string]string{"scene_id": sc.ID, "title": sc.Title, "verdict": sc.Speech, "summary": sc.SpeechSummary})
		}
	}
	return out
}
