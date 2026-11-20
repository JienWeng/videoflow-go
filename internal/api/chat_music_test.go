package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestAssistantAddsMusicToAnEpisodeAndOffersToRemakeItsVideos(t *testing.T) {
	s, _ := falServer(t, "done")
	answer := `{"reply":"ok","action":"set_music","episode_number":1,"music":"slow sparse piano"}`
	_, prompt := plannerSays(s, &answer)
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Idea: "end", Title: "The End", Summary: "end", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	one := renderableScene(t, s, &script.ID, "One", 1, "auto")
	two := renderableScene(t, s, &script.ID, "Two", 2, "auto")

	out := sendChat(t, s, "this needs music")
	if !strings.Contains(*prompt, `"episode_sound":[{"episode_number":1,"music":"none","speech":"","title":"The End","voice":"video model"}]`) {
		t.Errorf("the planner was not told how the episode's sound is made:\n%.400s", (*prompt)[strings.Index(*prompt, "episode_sound"):])
	}
	if out.Intent.Action != "render_scenes" || len(out.Intent.SceneIDs) != 2 || !strings.Contains(out.Intent.Reply, "“slow sparse piano”") || !strings.Contains(out.Intent.Reply, "2 scene videos need to be made again") {
		t.Fatalf("action %q scenes %v reply %q", out.Intent.Action, out.Intent.SceneIDs, out.Intent.Reply)
	}
	for _, id := range []string{one.ID, two.ID} {
		var preview struct {
			Clips []struct{ Prompt string }
		}
		w := call(t, s, "GET", "/scenes/"+id+"/video-prompt", "")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || len(preview.Clips) == 0 {
			t.Fatal(w.Code, w.Body.String())
		}
		if !strings.Contains(preview.Clips[0].Prompt, "(slow sparse piano plays quietly in the background") {
			t.Errorf("the next video of %s would not have the music:\n%s", id, preview.Clips[0].Prompt)
		}
	}

	answer = `{"reply":"ok","action":"set_music","episode_number":1,"music":"none"}`
	if out = sendChat(t, s, "actually no music"); !strings.Contains(out.Intent.Reply, "Music is now off for episode 1") {
		t.Fatalf("reply %q", out.Intent.Reply)
	}
	var preview struct {
		Clips []struct{ Prompt string }
	}
	json.Unmarshal(call(t, s, "GET", "/scenes/"+one.ID+"/video-prompt", "").Body.Bytes(), &preview)
	if len(preview.Clips) == 0 || !strings.Contains(preview.Clips[0].Prompt, "No BGM.") {
		t.Fatalf("music was not removed: %+v", preview)
	}
}

// The music picked with the story is what its scenes are made with.
func TestMusicChosenWithTheStoryReachesTheVideoPrompt(t *testing.T) {
	s, _ := falServer(t, "done")
	scene := renderableScene(t, s, nil, "One", 1, "auto")
	var doc map[string]any
	json.Unmarshal(scene.SceneJSON, &doc)
	doc["generation_brief"] = map[string]any{"idea": "x", "music": "warm acoustic guitar"}
	scene.SceneJSON = voiceJSON(doc)
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	revision, _ := s.sceneInputRevision(scene)
	project, _ := s.database.GetActiveProjectID()
	assets, _ := s.database.ListAssets(project)
	for i := range assets {
		if assets[i].Type == "storyboard" {
			assets[i].MetadataJSON = voiceJSON(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "ready"})
			s.database.UpdateAsset(&assets[i])
		}
	}
	w := call(t, s, "GET", "/scenes/"+scene.ID+"/video-prompt", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "(warm acoustic guitar plays quietly") {
		t.Fatal(w.Code, w.Body.String())
	}
}
