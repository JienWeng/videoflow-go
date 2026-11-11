package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func TestAssetReviewRequiresOwnershipAndExactRevision(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider calls") })
	s.router.Post("/assets/{id}/review", s.handleAssetReview)
	project, _ := s.database.GetActiveProjectID()
	asset := &models.Asset{ProjectID: &project, Name: "Generated reference", Type: "storyboard", MetadataJSON: json.RawMessage(`{"consistency_review":"pending","locked_traits":{"color":"red"}}`)}
	s.database.CreateAsset(asset)
	saved, _ := s.database.GetAsset(asset.ID)
	body := fmt.Sprintf(`{"approved":true,"expected_updated_at":%q}`, saved.UpdatedAt.Format(time.RFC3339Nano))
	result := call(t, s, "POST", "/assets/"+asset.ID+"/review", body)
	if result.Code != 200 || !strings.Contains(result.Body.String(), `"consistency_review":"approved"`) || !strings.Contains(result.Body.String(), `"color":"red"`) {
		t.Fatal(result.Code, result.Body.String())
	}
	if stale := call(t, s, "POST", "/assets/"+asset.ID+"/review", body); stale.Code != 409 {
		t.Fatal(stale.Code, stale.Body.String())
	}
	fresh, _ := s.database.GetAsset(asset.ID)
	reject := call(t, s, "POST", "/assets/"+asset.ID+"/review", fmt.Sprintf(`{"approved":false,"expected_updated_at":%q}`, fresh.UpdatedAt.Format(time.RFC3339Nano)))
	if reject.Code != 200 || !strings.Contains(reject.Body.String(), `"consistency_review":"rejected"`) {
		t.Fatal(reject.Code, reject.Body.String())
	}
	other, _ := s.database.CreateProject("Other", "")
	s.database.ActivateProject(other.ID)
	if cross := call(t, s, "POST", "/assets/"+asset.ID+"/review", body); cross.Code != 404 {
		t.Fatal(cross.Code, cross.Body.String())
	}
}
func TestLocalPipelineStopsForGeneratedReferenceReview(t *testing.T) {
	var videos atomic.Int32
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]}}}]}`))
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1,4],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		case "/images":
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		case "/videos":
			videos.Add(1)
			w.WriteHeader(500)
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Pipeline review")
	fake := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(fake, []byte(`#!/usr/bin/env python3
import sys,json
p=sys.stdin.read()
result={'prompt':'A red car'}
if 'scriptwriter' in p: result={'title':'Car','summary':'Car','scenes':[{'title':'One','summary':'Car arrives','duration':1,'aspect_ratio':'16:9'}]}
elif 'cinematographer' in p: result={'shots':[{'shot_order':1,'duration':1,'prompt':'Car','camera':'wide','movement':'still'}]}
print(json.dumps({'type':'thread.started','thread_id':'review-native'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps(result)}}))
print(json.dumps({'type':'turn.completed'}))
`), 0700)
	s.directorRunner.Codex = fake
	result := call(t, s, "POST", "/videos/generate", `{"idea":"A red car","scene_count":1,"target_duration":1,"aspect_ratio":"16:9"}`)
	if result.Code != 202 {
		t.Fatal(result.Code, result.Body.String())
	}
	var response struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(result.Body.Bytes(), &response)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, _ := s.database.GetOp(response.ID)
		if op.Status == "failed" {
			t.Fatal(op.Error)
		}
		if op.Status == "succeeded" {
			if !strings.Contains(string(op.ResultJSON), "awaiting_reference_review") {
				t.Fatal(string(op.ResultJSON))
			}
			assets, _ := s.database.ListAssets(project)
			if len(assets) != 1 || !strings.Contains(string(assets[0].MetadataJSON), `"consistency_review":"pending"`) {
				t.Fatal(assets)
			}
			jobs, _ := s.database.ListRenderJobs(project)
			if len(jobs) != 0 || videos.Load() != 0 {
				t.Fatal("paid video submitted before visual review", jobs, videos.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pipeline did not reach review checkpoint")
}
