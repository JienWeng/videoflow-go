package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// A render is uncertain (status reconciliation_required) when the video service may
// have taken the request but never said how it went. Two ways out, neither of which
// sends the request again:
//   - Check again asks the service about its own job. Finished or still going: the app
//     waits for it as usual (no new charge). Failed: the scene can be made again.
//     No answer: it stays uncertain and says why.
//   - Stop waiting closes it on the owner's word, accepting the old request may be billed.

// renderCheck is what Check again found, in words for the owner.
type renderCheck struct {
	JobID   string `json:"job_id"`
	SceneID string `json:"scene_id,omitempty"`
	Service string `json:"service"`
	// Outcome is "found" (the service has it; the app is fetching it), "failed" (the
	// service did not make it; the scene can be made again) or "unknown" (still no answer).
	Outcome string `json:"outcome"`
	Message string `json:"message"`
}

func videoServiceLabel(provider string) string {
	if host := costs.HostByName(provider); host != nil {
		return host.Label
	}
	return serviceLabel(provider)
}

func (s *Server) registerRenderRecoveryRoutes() {
	s.router.Post("/render-jobs/{id}/check", s.handleCheckRender)
	s.router.Post("/render-jobs/{id}/stop-waiting", s.handleStopWaitingRender)
	s.router.Post("/render-jobs/clear-failed", s.handleClearFailedRenders)
	s.router.Get("/scenes/{id}/video-prompt", s.handleScenePromptPreview)
}

// projectRenderJob loads a render job of the open project.
func (s *Server) projectRenderJob(w http.ResponseWriter, r *http.Request) (*models.RenderJob, bool) {
	project, err := s.activeProjectID(r.Context())
	job, e := s.database.GetRenderJob(chi.URLParam(r, "id"))
	if err != nil || e != nil || job.ProjectID == nil || *job.ProjectID != project {
		http.Error(w, "that video is not in the open project", 404)
		return nil, false
	}
	return job, true
}

func (s *Server) handleCheckRender(w http.ResponseWriter, r *http.Request) {
	if job, ok := s.projectRenderJob(w, r); ok {
		writeJSON(w, 200, s.checkUncertainRender(r.Context(), job))
	}
}

func (s *Server) handleStopWaitingRender(w http.ResponseWriter, r *http.Request) {
	if job, ok := s.projectRenderJob(w, r); ok {
		writeJSON(w, 200, s.stopWaitingRender(job))
	}
}

// checkUncertainRender asks the video service once about an uncertain job. It is free:
// it only reads the status of the service's own job. Running it twice is harmless.
func (s *Server) checkUncertainRender(ctx context.Context, job *models.RenderJob) renderCheck {
	service := videoServiceLabel(job.Provider)
	out := renderCheck{JobID: job.ID, Service: service}
	if job.SceneID != nil {
		out.SceneID = *job.SceneID
	}
	switch job.Status {
	case "pending", "running", "succeeded":
		out.Outcome, out.Message = "found", service+" has this video. Nothing new is charged."
		return out
	case "failed", "canceled":
		out.Outcome, out.Message = "failed", "This video was not made, so the scene can be made again."
		return out
	}
	if job.ProviderJobID == nil || *job.ProviderJobID == "" {
		out.Outcome = "unknown"
		out.Message = service + " never confirmed this request, so there is nothing to ask it about."
		return out
	}
	client, _, err := s.routes().Media(job.Provider, job.Model, job.ProviderEndpoint)
	if err != nil {
		out.Outcome, out.Message = "unknown", "VideoFlow cannot reach "+service+" because its key is missing. Add it in Settings, then press Check again."
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	status, _, err := client.GetVideo(ctx, *job.ProviderJobID)
	switch {
	case status == "failed" || status == "cancelled" || status == "canceled" || status == "expired":
		s.routes().NoteServiceAnswer(job.Provider, nil)
		if _, e := s.database.FailUncertainRender(job.ID, service+" did not make this video."); e != nil {
			out.Outcome, out.Message = "unknown", "VideoFlow could not save the answer. Press Check again."
			return out
		}
		out.Outcome, out.Message = "failed", service+" says it did not make this video, so the scene can be made again."
		return out
	case providers.NoSuchRequest(err):
		// The service answered, and it has no such request: nothing was made.
		s.routes().NoteServiceAnswer(job.Provider, nil)
		if _, e := s.database.FailUncertainRender(job.ID, service+" has no record of this request."); e != nil {
			out.Outcome, out.Message = "unknown", "VideoFlow could not save the answer. Press Check again."
			return out
		}
		out.Outcome, out.Message = "failed", service+" has no record of this request, so it did not make this video. The scene can be made again."
		return out
	case err != nil:
		s.routes().NoteServiceAnswer(job.Provider, err)
		out.Outcome = "unknown"
		switch {
		case providers.NoCredit(err):
			out.Message = providers.OutOfCredit(service).Message + " Until then " + service + " will not say whether it made this video."
		case providers.InvalidKey(err):
			out.Message = providers.RejectedKey(service).Message + " Then press Check again."
		default:
			out.Message = service + " did not answer just now. Press Check again in a few minutes."
		}
		return out
	}
	s.routes().NoteServiceAnswer(job.Provider, nil)
	if _, e := s.database.ResumeUncertainRender(job.ID); e != nil {
		out.Outcome, out.Message = "unknown", "VideoFlow could not save the answer. Press Check again."
		return out
	}
	s.workers.Enqueue(job.ID)
	out.Outcome = "found"
	if status == "completed" {
		out.Message = service + " finished this video. It is being saved now, and nothing new is charged."
	} else {
		out.Message = service + " is still making this video. It appears here when it is done, and nothing new is charged."
	}
	return out
}

// stopWaitingRender closes an uncertain job on the owner's word so the scene can be made
// again. The earlier request might still be billed by the service.
func (s *Server) stopWaitingRender(job *models.RenderJob) renderCheck {
	service := videoServiceLabel(job.Provider)
	out := renderCheck{JobID: job.ID, Service: service, Outcome: "failed", Message: "Stopped waiting. The scene can be made again."}
	if job.SceneID != nil {
		out.SceneID = *job.SceneID
	}
	if job.Status != "reconciliation_required" {
		return s.checkUncertainRender(context.Background(), job)
	}
	if _, err := s.database.FailUncertainRender(job.ID, "Stopped waiting for "+service+". The earlier request might still be billed."); err != nil {
		out.Outcome, out.Message = "unknown", "VideoFlow could not save that. Press Stop waiting again."
	}
	return out
}

// handleClearFailedRenders puts away the open project's failed video attempts. Free;
// nothing is deleted and every scene can still be made again.
func (s *Server) handleClearFailedRenders(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 500)
		return
	}
	n, err := s.database.ClearFailedRenders(project)
	if err != nil {
		http.Error(w, "could not clear the failed videos", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"cleared": n, "message": clearedFailedReply(n)})
}

func clearedFailedReply(n int) string {
	switch n {
	case 0:
		return "There are no failed video attempts left to clear."
	case 1:
		return "Cleared 1 failed video attempt. Your scenes and pictures are untouched, and the scene can be made again."
	}
	return fmt.Sprintf("Cleared %d failed video attempts. Your scenes and pictures are untouched, and the scenes can be made again.", n)
}
