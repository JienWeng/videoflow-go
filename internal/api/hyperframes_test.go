package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestCompositionRevisionOwnershipAndRunFence(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call") })
	saved := call(t, s, "POST", "/compositions", `{"version":1,"width":640,"height":360,"fps":24,"duration":2,"clips":[],"audio":[],"captions":[{"id":"cue-1","utterance_id":"line-1","start":0.2,"end":1.5,"text":"Approved text"}]}`)
	if saved.Code != 201 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	var m struct {
		Revision string `json:"revision"`
	}
	if e := json.Unmarshal(saved.Body.Bytes(), &m); e != nil || m.Revision == "" {
		t.Fatal(saved.Body.String())
	}
	bad := call(t, s, "POST", "/compositions/"+m.Revision+"/run", `{"operation":"render","revision":"stale"}`)
	if bad.Code != 409 {
		t.Fatal(bad.Code, bad.Body.String())
	}
	blocked := call(t, s, "POST", "/compositions/"+m.Revision+"/run", `{"operation":"publish","revision":"`+m.Revision+`"}`)
	if blocked.Code != 422 || !strings.Contains(blocked.Body.String(), "unsupported local operation") {
		t.Fatal(blocked.Code, blocked.Body.String())
	}
	other, e := s.database.CreateProject("Other", "isolated")
	if e != nil {
		t.Fatal(e)
	}
	s.database.ActivateProject(other.ID)
	got := call(t, s, "GET", "/compositions/"+m.Revision, "")
	if got.Code != 404 {
		t.Fatal(got.Code, got.Body.String())
	}
}

func TestCompositionSourcesResolvePublicPathsAndOwnedOutputs(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider") })
	project, _ := s.database.GetActiveProjectID()
	os.MkdirAll(filepath.Join(s.cfg.StorageRoot, "assets"), 0700)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "assets", "existing.mp4"), []byte("local"), 0600)
	a := &models.Asset{ID: "asset-existing", ProjectID: &project, Type: "video_reference", Name: "Existing upload", FilePath: "storage/assets/existing.mp4", TagsJSON: []byte("[]"), MetadataJSON: []byte("{}")}
	if e := s.database.CreateUploadedAsset(a); e != nil {
		t.Fatal(e)
	}
	job := &models.RenderJob{ProjectID: &project, Provider: "local", Model: "fixture", Status: "succeeded", RequestJSON: []byte("{}")}
	out := &models.RenderOutput{VideoPath: "storage/assets/existing.mp4"}
	if e := s.database.SaveLocalOutput(job, out); e != nil {
		t.Fatal(e)
	}
	paths, items, e := s.compositionSources(project)
	if e != nil {
		t.Fatal(e)
	}
	if paths[a.ID] != filepath.Join(s.cfg.StorageRoot, "assets", "existing.mp4") || paths["output-"+out.ID] == "" || len(items) != 2 {
		t.Fatalf("sources: %#v %#v", paths, items)
	}
	response := call(t, s, "GET", "/compositions/sources", "")
	var sources []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &sources); err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source["media_kind"] != "video" {
			t.Fatalf("UI cannot distinguish visual and audio sources: %v", source)
		}
	}
	other, _ := s.database.CreateProject("Other", "isolation")
	paths, items, e = s.compositionSources(other.ID)
	if e != nil || len(paths) != 0 || len(items) != 0 {
		t.Fatal("cross-project sources", paths, items, e)
	}
}

func TestCompositionPublicSourceRenderIntegration(t *testing.T) {
	fixture := os.Getenv("VIDEOFLOW_HF_TEST_MEDIA")
	if fixture == "" {
		t.Skip("opt-in real local composition integration")
	}
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("unexpected provider call") })
	project, _ := s.database.GetActiveProjectID()
	data, e := os.ReadFile(fixture)
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(s.cfg.StorageRoot, "outputs"), 0700)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "outputs", "existing.mp4"), data, 0600)
	job := &models.RenderJob{ProjectID: &project, Provider: "local", Model: "fixture", Status: "succeeded", RequestJSON: []byte("{}")}
	out := &models.RenderOutput{VideoPath: "storage/outputs/existing.mp4"}
	if e = s.database.SaveLocalOutput(job, out); e != nil {
		t.Fatal(e)
	}
	body := fmt.Sprintf(`{"version":1,"width":640,"height":360,"fps":24,"duration":2,"clips":[{"id":"source","asset_id":%q,"start":0,"trim":0,"duration":2,"speed":1}],"audio":[],"captions":[{"id":"cue-1","utterance_id":"line-1","start":0.2,"end":1.5,"text":"Approved script"}]}`, "output-"+out.ID)
	saved := call(t, s, "POST", "/compositions", body)
	if saved.Code != 201 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	var m struct {
		Revision string `json:"revision"`
	}
	json.Unmarshal(saved.Body.Bytes(), &m)
	for _, operation := range []string{"lint", "snapshot", "render"} {
		got := call(t, s, "POST", "/compositions/"+m.Revision+"/run", fmt.Sprintf(`{"revision":%q,"operation":%q}`, m.Revision, operation))
		if got.Code != 200 {
			t.Fatal(operation, got.Code, got.Body.String())
		}
		if operation == "render" {
			var result struct {
				Asset models.Asset `json:"asset"`
			}
			if e = json.Unmarshal(got.Body.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			if result.Asset.ID == "" || !strings.Contains(string(result.Asset.MetadataJSON), "output-"+out.ID) {
				t.Fatal("missing derived provenance", got.Body.String())
			}
			if stat, e := os.Stat(result.Asset.FilePath); e != nil || stat.Size() == 0 {
				t.Fatal("no derived media", e)
			}
		}
	}
}
