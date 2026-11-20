package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

// finishedVideo gives a scene a finished video and returns its output id.
func finishedVideo(t *testing.T, s *Server, scene models.Scene) string {
	t.Helper()
	job := &models.RenderJob{ProjectID: scene.ProjectID, SceneID: &scene.ID, Provider: "argolink", Model: "seedance-2.0", Status: "succeeded"}
	if e := s.database.CreateRenderJob(job); e != nil {
		t.Fatal(e)
	}
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/outputs/" + job.ID + "-0.mp4"}
	if e := s.database.CreateRenderOutput(out); e != nil {
		t.Fatal(e)
	}
	return out.ID
}

// exported saves a final video joined from the given scene videos.
func exported(t *testing.T, s *Server, project string, outputs ...string) string {
	t.Helper()
	sources := []map[string]string{}
	for _, id := range outputs {
		sources = append(sources, map[string]string{"asset_id": "output-" + id})
	}
	meta, _ := json.Marshal(map[string]any{"composition_revision": "rev", "source_assets": sources})
	asset := &models.Asset{ProjectID: &project, Type: "output_video", Name: "Edited video.mp4", FilePath: "/data/storage/assets/final.mp4", MetadataJSON: meta}
	if e := s.database.CreateAsset(asset); e != nil {
		t.Fatal(e)
	}
	return asset.ID
}

func TestCanvasShowsWhichSceneVideosAFinalVideoJoins(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	scenes := storyScenes(t, s, "One", "Two")
	one, two := finishedVideo(t, s, scenes[0]), finishedVideo(t, s, scenes[1])
	exported(t, s, project, one, two) // an older export of the same episode
	final := exported(t, s, project, one, two)

	var graph struct {
		Nodes []struct {
			ID, Type, Label string
			Data            map[string]any
		}
		Edges []struct{ Source, Target, Label string }
	}
	if w := call(t, s, "GET", "/graph", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &graph) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	finals, labels := []string{}, map[string]string{}
	for _, n := range graph.Nodes {
		labels[n.ID] = n.Label
		switch n.Type {
		case "final_video":
			finals = append(finals, n.ID)
			if n.Label != "Episode 1 · Pup" || fmt.Sprint(n.Data["scene_titles"]) != "[One Two]" {
				t.Fatalf("final video is named %q and joins %v", n.Label, n.Data["scene_titles"])
			}
		case "scene":
			if n.Data["episode_number"] != float64(1) || n.Data["episode_title"] != "Pup" {
				t.Fatalf("scene %s does not say its episode: %v", n.Label, n.Data)
			}
		}
	}
	if len(finals) != 1 || finals[0] != final {
		t.Fatalf("final videos shown: %v, want only the newest %s", finals, final)
	}
	if labels[one] != "One · video" || labels[two] != "Two · video" {
		t.Fatalf("scene videos are named %q and %q", labels[one], labels[two])
	}
	joined := []string{}
	for _, e := range graph.Edges {
		if e.Label == "joined" && e.Target == final {
			joined = append(joined, e.Source)
		}
	}
	if len(joined) != 2 || !strings.Contains(strings.Join(joined, ","), one) || !strings.Contains(strings.Join(joined, ","), two) {
		t.Fatalf("joined links: %v", joined)
	}
}

func TestJoiningAnEpisodeNeverMakesAVideo(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	scenes := storyScenes(t, s, "One", "Two")
	finishedVideo(t, s, scenes[0])
	w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"script_id":%q,"join_only":true}`, *scenes[0].ScriptID))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "“Two” has no finished video yet") {
		t.Fatal(w.Code, w.Body.String())
	}
	if jobs, _ := s.database.ListRenderJobs(project); len(jobs) != 1 {
		t.Fatalf("%d render jobs, want only the finished one", len(jobs))
	}
}

// Selecting a final video on the canvas, or something that no longer exists, must never
// stop a chat message from going through.
func TestChatAcceptsASelectedFinalVideoAndIgnoresAStaleSelection(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"The voice came from the video model.","action":"none"}`
	_, prompt := plannerSays(s, &answer)
	project, _ := s.database.GetActiveProjectID()
	scenes := storyScenes(t, s, "One")
	final := exported(t, s, project, finishedVideo(t, s, scenes[0]))

	w := call(t, s, "POST", "/chat", fmt.Sprintf(`{"message":"is the voice from the video model?","selected":{"id":%q,"kind":"final_video"}}`, final))
	if w.Code != 200 || !strings.Contains(*prompt, "the finished, joined video of an episode") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, stale := range []string{`{"id":"asset_gone","kind":"final_video"}`, `{"id":"scene_gone","kind":"scene"}`, `{"id":"x","kind":"something_new"}`} {
		if w = call(t, s, "POST", "/chat", `{"message":"hello","selected":`+stale+`}`); w.Code != 200 {
			t.Fatalf("selection %s blocked the message: %d %s", stale, w.Code, w.Body.String())
		}
	}
}
