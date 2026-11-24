package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

// A story made in several clips gets its music as one bed in the edit: setting it
// remakes no video, and the clips are told to make no music of their own.
func TestAssistantAddsMusicToAnEpisodeAsOneBedWithoutRemakingVideos(t *testing.T) {
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
	if !strings.Contains(*prompt, `"episode_sound":[{`) {
		t.Errorf("the planner was not told how the episode's sound is made:\n%.400s", *prompt)
	}
	if out.Intent.Action == "render_scenes" || len(out.Intent.SceneIDs) != 0 || !strings.Contains(out.Intent.Reply, "“slow sparse piano”: one track under the whole film") || !strings.Contains(out.Intent.Reply, "nothing is made again") {
		t.Fatalf("action %q scenes %v reply %q", out.Intent.Action, out.Intent.SceneIDs, out.Intent.Reply)
	}
	plan, clips := s.storySound(context.Background(), project, script.ID)
	if clips != 2 || plan.Music.Mode != musicBed || plan.Music.Prompt != "slow sparse piano" || plan.ClipAudio != clipDialogueAmbience || plan.DecidedBy != decidedOwner {
		t.Fatalf("clips %d plan %+v", clips, plan)
	}
	for _, id := range []string{one.ID, two.ID} {
		preview := sceneClipPrompts(t, s, id, false)
		if len(preview) == 0 {
			t.Fatal("no clips for ", id)
		}
		if strings.Contains(preview[0].Prompt, "piano") || !strings.Contains(preview[0].Prompt, "No BGM; generate only voices, environmental sounds and action sounds.") {
			t.Errorf("the clip of %s would make its own music:\n%s", id, preview[0].Prompt)
		}
	}

	answer = `{"reply":"ok","action":"set_music","episode_number":1,"music":"none"}`
	if out = sendChat(t, s, "actually no music"); !strings.Contains(out.Intent.Reply, "Music is now off for episode 1") {
		t.Fatalf("reply %q", out.Intent.Reply)
	}
	if plan, _ = s.storySound(context.Background(), project, script.ID); plan.Music.Mode != musicNone {
		t.Fatalf("music was not removed: %+v", plan)
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
	if preview := sceneClipPrompts(t, s, scene.ID, false); len(preview) == 0 || !strings.Contains(preview[0].Prompt, "(warm acoustic guitar plays quietly") {
		t.Fatal(preview)
	}
}
