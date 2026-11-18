package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

var referenceVideoMu sync.Mutex

type generationReference struct {
	AssetID string `json:"asset_id"`
	Kind    string `json:"kind"`
	Purpose string `json:"purpose"`
}
type referenceVideoRequest struct {
	Prompt     string                `json:"prompt"`
	Duration   int                   `json:"duration"`
	Aspect     string                `json:"aspect_ratio"`
	Resolution string                `json:"resolution"`
	Audio      bool                  `json:"generate_audio"`
	References []generationReference `json:"references"`
	Confirmed  string                `json:"confirmed_plan_id"`
}

func (s *Server) referenceVideoPayload(ctx context.Context, project string, b referenceVideoRequest) (map[string]any, string, error) {
	if strings.TrimSpace(b.Prompt) == "" || len(b.Prompt) > 12000 || b.Duration < 4 || b.Duration > 15 || len(b.References) == 0 || len(b.References) > 12 {
		return nil, "", fmt.Errorf("provide a prompt, 4–15 seconds and 1–12 references")
	}
	payload := map[string]any{"model": providers.FalNativeVoiceModel, "prompt": b.Prompt, "duration": b.Duration, "aspect_ratio": b.Aspect, "resolution": b.Resolution, "generate_audio": b.Audio}
	total := 0
	videoDuration, audioDuration := 0.0, 0.0
	seen := map[string]bool{}
	counts := map[string]int{}
	versions := []string{}
	notes := []string{}
	for _, ref := range b.References {
		if seen[ref.AssetID] || len(ref.Purpose) > 200 {
			return nil, "", fmt.Errorf("choose each reference once with a concise role")
		}
		seen[ref.AssetID] = true
		asset, e := s.database.GetAsset(ref.AssetID)
		if e != nil || asset.ProjectID == nil || *asset.ProjectID != project || !models.ReferenceApproved(asset.MetadataJSON) {
			return nil, "", fmt.Errorf("reference is missing, excluded or outside this project")
		}
		versions = append(versions, asset.ID+":"+asset.UpdatedAt.String())
		path, e := s.storagePath(asset.FilePath)
		if e != nil {
			return nil, "", e
		}
		file, e := os.Open(path)
		if e != nil {
			return nil, "", e
		}
		data, e := io.ReadAll(io.LimitReader(file, 50<<20+1))
		file.Close()
		if e != nil {
			return nil, "", e
		}
		if len(data) > 50<<20 {
			return nil, "", fmt.Errorf("reference file exceeds 50 MB")
		}
		total += len(data)
		if total > 64<<20 {
			return nil, "", fmt.Errorf("combined reference files exceed the studio's 64 MB limit")
		}
		mime := ""
		key := ""
		tag := ""
		switch ref.Kind {
		case "image":
			if len(data) > 30<<20 {
				return nil, "", fmt.Errorf("image exceeds 30 MB")
			}
			if referenceIsVideo(data) {
				return nil, "", fmt.Errorf("choose image mode for an actual image")
			}
			if _, _, e = referenceImages(ctx, data); e != nil {
				return nil, "", fmt.Errorf("image reference is invalid")
			}
			mime = http.DetectContentType(data)
			key = "image_urls"
			tag = "Image"
		case "video":
			ext := strings.ToLower(filepath.Ext(path))
			if (ext != ".mp4" && ext != ".mov") || !referenceIsVideo(data) {
				return nil, "", fmt.Errorf("generation video references must be MP4 or MOV")
			}
			duration, width, height, _, e := composition.Probe(ctx, path)
			if e != nil {
				return nil, "", e
			}
			pixels := width * height
			if pixels < 409600 || pixels > 927408 {
				return nil, "", fmt.Errorf("video reference must be approximately 480p–720p; trim/resize it first")
			}
			videoDuration += duration
			mime = "video/mp4"
			key = "video_urls"
			tag = "Video"
		case "audio":
			ext := strings.ToLower(filepath.Ext(path))
			if (ext != ".mp3" && ext != ".wav") || len(data) > 15<<20 {
				return nil, "", fmt.Errorf("audio reference must be MP3/WAV under 15 MB")
			}
			duration, _, _, audio, e := composition.Probe(ctx, path)
			if e != nil || !audio {
				return nil, "", fmt.Errorf("audio reference cannot be decoded")
			}
			audioDuration += duration
			mime = "audio/mpeg"
			if ext == ".wav" {
				mime = "audio/wav"
			}
			key = "audio_urls"
			tag = "Audio"
		default:
			return nil, "", fmt.Errorf("choose image, video or audio reference roles")
		}
		counts[ref.Kind]++
		values, _ := payload[key].([]string)
		payload[key] = append(values, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
		notes = append(notes, fmt.Sprintf("@%s%d: %s", tag, counts[ref.Kind], ref.Purpose))
	}
	if counts["video"] > 0 && (videoDuration < 2 || videoDuration > 15.05) {
		return nil, "", fmt.Errorf("combined reference video duration must be 2–15 seconds")
	}
	if audioDuration > 15.05 {
		return nil, "", fmt.Errorf("combined reference audio must not exceed 15 seconds")
	}
	payload["prompt"] = b.Prompt + "\nReference roles:\n" + strings.Join(notes, "\n") + "\nProject visual direction: " + s.stylePrompt(&project)
	client, _, e := s.routes().Media("fal", providers.FalNativeVoiceModel, "")
	if e != nil {
		return nil, "", e
	}
	if e = client.ValidateVideo(ctx, payload); e != nil {
		return nil, "", e
	}
	return payload, compositionHash(voiceJSON(map[string]any{"payload": payload, "versions": versions})), nil
}
func (s *Server) handleReferenceVideo(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	var b referenceVideoRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20000))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&b) != nil {
		http.Error(w, "invalid reference-video request", 400)
		return
	}
	referenceVideoMu.Lock()
	defer referenceVideoMu.Unlock()
	payload, id, e := s.referenceVideoPayload(r.Context(), project, b)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	var existing *models.RenderJob
	jobs, _ := s.database.ListRenderJobs(project)
	for i := range jobs {
		var previous struct {
			Plan string `json:"reference_plan_id"`
		}
		json.Unmarshal(jobs[i].RequestJSON, &previous)
		if previous.Plan == id && jobs[i].Status != "failed" && jobs[i].Status != "canceled" {
			existing = &jobs[i]
			break
		}
	}
	if strings.HasSuffix(r.URL.Path, "/plan") {
		calls := 1
		if existing != nil {
			calls = 0
		}
		writeJSON(w, 200, map[string]any{"id": id, "model": providers.FalNativeVoiceModel, "new_calls": calls, "generated_seconds": calls * b.Duration, "existing": existing, "references": b.References})
		return
	}
	if b.Confirmed != id {
		http.Error(w, "references or settings changed; check the generation plan again", 409)
		return
	}
	if existing != nil {
		if existing.Status == "reconciliation_required" {
			http.Error(w, "provider outcome is uncertain; resolve the existing job in Activity before retrying", 409)
			return
		}
		writeJSON(w, 202, map[string]any{"job_id": existing.ID, "reused": true})
		return
	}
	payload["reference_plan_id"] = id
	if b.Audio {
		payload["audio_mode"] = "native"
	} else {
		payload["audio_mode"] = "silent"
	}
	stage := "submitted"
	job := &models.RenderJob{ProjectID: &project, Provider: "fal", Model: providers.FalNativeVoiceModel, Status: "pending", Stage: &stage, RequestJSON: voiceJSON(payload)}
	binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
	binding.Selection = directorSelection{}
	for _, ref := range b.References {
		binding.Selection.AssetIDs = append(binding.Selection.AssetIDs, ref.AssetID)
	}
	ctx := context.WithValue(r.Context(), directorBindingKey{}, binding)
	if e = s.prepareDirectorVideoJob(ctx, job); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e = s.database.CreateRenderJob(job); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	s.workers.Enqueue(job.ID)
	writeJSON(w, 202, map[string]any{"job_id": job.ID, "reused": false})
}
