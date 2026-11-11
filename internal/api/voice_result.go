package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

func (s *Server) handleRefreshVoice(w http.ResponseWriter, r *http.Request) {
	task, ok := s.voiceTaskFromRequest(w, r)
	if !ok {
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	task, err := s.database.GetVoiceTask(task.ProjectID, task.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if task.Status == "completed" || task.Status == "review_required" {
		writeJSON(w, 200, task)
		return
	}
	if task.UpstreamID == "" {
		http.Error(w, "no upstream receipt; reconcile provider activity, never submit again", 409)
		return
	}
	client, err := s.voiceClient(task)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if task.Status != "result_ready" {
		status, result, err := client.VoiceResult(r.Context(), task.Model, task.UpstreamID)
		if err != nil {
			task.Error = "Result retrieval failed; refresh only, no paid resubmission. " + err.Error()
			if status == "failed" {
				task.Status = "failed"
			}
			s.database.UpdateVoiceTask(task)
			writeJSON(w, 200, task)
			return
		}
		if status != "completed" {
			task.Status = status
			task.Error = ""
			if err = s.database.UpdateVoiceTask(task); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeJSON(w, 200, task)
			return
		}
		task.Status = "result_ready"
		task.Result = result
		task.Error = ""
		// Save the provider result before downloading/importing local artifacts. A
		// restart can finish local work without reissuing billable generation.
		if err = s.database.UpdateVoiceTask(task); err != nil {
			http.Error(w, "could not save result; upstream receipt retained", 503)
			return
		}
	}
	if err = s.finishVoiceResult(r.Context(), client, task); err != nil {
		task.Error = err.Error()
	}
	if err = s.database.UpdateVoiceTask(task); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, task)
}
func voiceBinding(task *db.VoiceTask, kind string) db.VoiceBinding {
	for _, b := range task.Bindings {
		if b.Kind == kind {
			return b
		}
	}
	return db.VoiceBinding{}
}
func (s *Server) finishVoiceResult(ctx context.Context, client *providers.FalClient, task *db.VoiceTask) error {
	var result map[string]json.RawMessage
	if err := json.Unmarshal(task.Result, &result); err != nil {
		return err
	}
	switch task.Kind {
	case "clone":
		var id string
		json.Unmarshal(result["custom_voice_id"], &id)
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("fal returned no saved voice ID; keep receipt for provider review")
		}
		task.Status = "review_required"
		task.Error = "Listen to the provider preview, then explicitly accept the saved voice for this character. Use the cloned voice with TTS within 7 days to retain it."
	case "alignment":
		binding := voiceBinding(task, "script")
		doc, err := s.database.GetVoiceDocument(task.ProjectID, "script", binding.ID)
		if err != nil {
			return err
		}
		if doc.Revision != binding.Revision {
			return fmt.Errorf("script changed; retained alignment is stale and cannot be applied")
		}
		var script SpokenScript
		json.Unmarshal(doc.Data, &script)
		cues, err := alignSpokenLines(script.Lines, task.Result)
		if err != nil {
			task.Status = "review_required"
			task.Error = err.Error()
			return nil
		}
		// Keep provider words/loss for drift review alongside the editable candidate.
		result["segments"] = voiceJSON(cues)
		result["review_warning"] = voiceJSON("Forced alignment supplies timing; it does not prove the speech matches the script. Listen for omissions and wording drift before applying.")
		task.Result = voiceJSON(result)
		task.Status = "review_required"
	case "speech", "native_reference", "lipsync":
		video := task.Kind != "speech"
		key, ext, limit := "audio", ".mp3", int64(32<<20)
		if video {
			key, ext, limit = "video", ".mp4", 256<<20
		}
		var file struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(result[key], &file) != nil || file.URL == "" {
			return fmt.Errorf("fal result has no %s URL; upstream receipt retained", key)
		}
		artifactID := "voice_" + task.ID
		// Deterministic IDs make recovery after a save/result-record gap idempotent.
		if video {
			if output, err := s.database.GetRenderOutput(artifactID); err == nil {
				result["output_id"] = voiceJSON(output.ID)
				task.Result = voiceJSON(result)
				task.Status = "completed"
				return nil
			}
		} else {
			if asset, err := s.database.GetAsset(artifactID); err == nil {
				result["asset_id"] = voiceJSON(asset.ID)
				task.Result = voiceJSON(result)
				task.Status = "completed"
				return nil
			}
		}
		data, err := client.DownloadVoiceResult(ctx, file.URL, limit)
		if err != nil {
			return err
		}
		folder := "assets"
		if video {
			folder = "outputs"
		}
		dir := filepath.Join(s.cfg.StorageRoot, folder)
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		temp, err := os.CreateTemp(dir, ".voice-*")
		if err != nil {
			return err
		}
		name := temp.Name()
		defer os.Remove(name)
		if _, err = temp.Write(data); err != nil {
			temp.Close()
			return err
		}
		if err = temp.Close(); err != nil {
			return err
		}
		duration, err := voiceRequireMedia(ctx, name, key)
		if err != nil {
			return fmt.Errorf("provider %s result is not valid local media: %w", key, err)
		}
		destination := filepath.Join(dir, artifactID+ext)
		if err = os.Rename(name, destination); err != nil {
			return err
		}
		public := filepath.ToSlash(filepath.Join("storage", folder, artifactID+ext))
		scriptBinding := voiceBinding(task, "script")
		meta := map[string]any{"voice_task_id": task.ID, "upstream_id": task.UpstreamID, "model": task.Model, "source_bindings": task.Bindings, "script_id": scriptBinding.ID, "script_revision": scriptBinding.Revision, "duration_seconds": duration, "review_required": true}
		if !video {
			utterance := voiceBinding(task, "utterance")
			meta["line_id"] = utterance.ID
			doc, err := s.database.GetVoiceDocument(task.ProjectID, "script", scriptBinding.ID)
			if err != nil {
				return err
			}
			var script SpokenScript
			json.Unmarshal(doc.Data, &script)
			if doc.Revision == scriptBinding.Revision {
				for _, line := range script.Lines {
					if line.ID == utterance.ID {
						meta["character_id"] = line.CharacterID
						meta["lip_sync_required"] = line.Visible
						break
					}
				}
			}
			character := voiceBinding(task, "profile").ID
			asset := &models.Asset{ID: artifactID, ProjectID: &task.ProjectID, CharacterID: &character, Type: "audio", Name: "Saved character speech", FilePath: public, MetadataJSON: voiceJSON(meta)}
			if err = s.database.CreateAsset(asset); err != nil {
				return err
			}
			result["asset_id"] = voiceJSON(asset.ID)
		} else {
			job := &models.RenderJob{ID: artifactID, ProjectID: &task.ProjectID, Provider: "fal", Model: task.Model, ProviderEndpoint: task.Endpoint, ProviderJobID: &task.UpstreamID, Status: "succeeded", RequestJSON: voiceJSON(meta), ResponseJSON: task.Result}
			output := &models.RenderOutput{ID: artifactID, VideoPath: public, QAJSON: voiceJSON(map[string]any{"review_required": true, "voice_identity": "Listen and inspect lip synchronization before reuse", "source_bindings": task.Bindings})}
			if err = s.database.SaveLocalOutput(job, output); err != nil {
				return err
			}
			result["output_id"] = voiceJSON(output.ID)
		}
		result["local_path"] = voiceJSON(public)
		task.Result = voiceJSON(result)
		task.Status = "completed"
	default:
		return fmt.Errorf("unsupported task kind")
	}
	return nil
}
func (s *Server) handleAcceptVoice(w http.ResponseWriter, r *http.Request) {
	task, ok := s.voiceTaskFromRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Reviewed bool `json:"reviewed"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if !body.Reviewed || task.Kind != "clone" || task.Status != "review_required" {
		http.Error(w, "review a completed voice creation before accepting", 409)
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if err := s.checkVoiceBindings(task); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	binding := voiceBinding(task, "profile")
	doc, err := s.database.GetVoiceDocument(task.ProjectID, "profile", binding.ID)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	var profile VoiceProfile
	json.Unmarshal(doc.Data, &profile)
	var result struct {
		VoiceID string `json:"custom_voice_id"`
	}
	json.Unmarshal(task.Result, &result)
	if result.VoiceID == "" {
		http.Error(w, "provider result contains no saved voice", 409)
		return
	}
	profile.VoiceID = result.VoiceID
	profile.Approved = true
	saved, err := s.database.SaveVoiceDocument(task.ProjectID, "profile", binding.ID, binding.Revision, voiceJSON(profile))
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	task.Status = "completed"
	task.Error = ""
	if err = s.database.UpdateVoiceTask(task); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, saved)
}
func (s *Server) handleApplyScriptCaptions(w http.ResponseWriter, r *http.Request) {
	task, ok := s.voiceTaskFromRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Reviewed bool `json:"reviewed"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if !body.Reviewed || task.Kind != "alignment" || task.Status != "review_required" {
		http.Error(w, "listen for omissions/drift and review aligned cues first", 409)
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if err := s.checkVoiceBindings(task); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	var result struct {
		Segments []models.CaptionSegment `json:"segments"`
	}
	if json.Unmarshal(task.Result, &result) != nil || len(result.Segments) == 0 {
		http.Error(w, "alignment has no valid cues; review drift and explicitly prepare corrected work", 409)
		return
	}
	outBinding := voiceBinding(task, "output")
	out, err := s.voiceOwnedOutput(task.ProjectID, outBinding.ID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out.CaptionsJSON = voiceJSON(map[string]any{"segments": result.Segments, "style": "clean", "source": "approved_script", "alignment_task_id": task.ID, "source_bindings": task.Bindings, "reviewed": true})
	out.CaptionedPath = nil
	if err = s.database.UpdateRenderOutput(out); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"output_id": out.ID, "segments": result.Segments, "status": "applied", "editor_url": "/editor/" + out.ID})
}
func (s *Server) handleExportScriptCaptions(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	out, err := s.voiceOwnedOutput(project, chi.URLParam(r, "output"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	var captions struct {
		Segments []models.CaptionSegment `json:"segments"`
		Bindings []db.VoiceBinding       `json:"source_bindings"`
	}
	if json.Unmarshal(out.CaptionsJSON, &captions) != nil {
		json.Unmarshal(out.CaptionsJSON, &captions.Segments)
	}
	if len(captions.Bindings) > 0 {
		if err = s.checkVoiceBindings(&db.VoiceTask{ProjectID: project, Bindings: captions.Bindings}); err != nil {
			http.Error(w, "caption source is stale: "+err.Error(), 409)
			return
		}
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "srt"
	}
	body, err := exportScriptCaptions(captions.Segments, format)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	contentType := "application/x-subrip"
	if format == "vtt" {
		contentType = "text/vtt"
	}
	if format == "json" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="captions.%s"`, format))
	w.Write([]byte(body))
}
