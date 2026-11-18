package api

import (
	"encoding/json"
	"strings"
	"videoflow-go/internal/models"
)

// Only pass bounded status facts to chat. Provider responses, prompts, and credentials
// can appear in raw job payloads and errors.
func recoveryHint(message *string) string {
	if message == nil {
		return ""
	}
	text := strings.ToLower(*message)
	for _, rule := range []struct{ fragment, label string }{
		{"may have been attempted", "provider submission uncertain"},
		{"not in this project", "project selection mismatch"},
		{"invalid duration", "invalid timing"},
		{"requires review", "session needs review"},
		{"not installed", "local CLI unavailable"},
		{"sign in", "check sign-in"},
		{"unauthorized", "check provider authentication"},
		{"rate limit", "provider rate limit"},
		{"quota", "check provider quota"},
		{"storyboard", "check storyboard operation in Activity"},
	} {
		if strings.Contains(text, rule.fragment) {
			return rule.label
		}
	}
	return ""
}

func (s *Server) chatActivity(project string, jobs []models.RenderJob) map[string]any {
	ops, err := s.database.ListOps(project, 12)
	operations := []map[string]any{}
	renders := []map[string]any{}
	for _, op := range ops {
		if op.ProjectID == nil || *op.ProjectID != project {
			continue
		}
		item := map[string]any{"id": op.ID, "kind": op.Kind, "status": op.Status, "scene_id": op.SceneID, "updated_at": op.UpdatedAt, "diagnostic_hint": recoveryHint(op.Error)}
		if op.Kind == "storyboard_audit" {
			var result struct{ Status string }
			if json.Unmarshal(op.ResultJSON, &result) == nil {
				switch result.Status {
				case "matched", "repaired", "needs_repair", "needs_attention":
					item["visual_verdict"] = result.Status
				}
			}
		}
		operations = append(operations, item)
	}
	for _, job := range jobs {
		if job.ProjectID == nil || *job.ProjectID != project {
			continue
		}
		renders = append(renders, map[string]any{"id": job.ID, "scene_id": job.SceneID, "status": job.Status, "updated_at": job.UpdatedAt, "provider_job_recorded": job.ProviderJobID != nil && *job.ProviderJobID != "", "diagnostic_hint": recoveryHint(job.Error)})
		if len(renders) == 12 {
			break
		}
	}
	return map[string]any{"operations": operations, "operation_history_available": err == nil, "render_jobs": renders}
}
