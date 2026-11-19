package api

import (
	"encoding/json"
	"testing"

	"videoflow-go/internal/models"
)

func TestCostsBreakDownBySceneServiceAndEpisodeAndSurviveDeletion(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Title: "First Noodles", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	scene := &models.Scene{ProjectID: &project, ScriptID: &script.ID, SceneOrder: 1, Title: "Kitchen", Duration: 10, AspectRatio: "16:9", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	job := func(status string) {
		j := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "fal", Model: "bytedance/seedance-2.0/fast/image-to-video", Status: status, RequestJSON: []byte(`{"duration":10,"resolution":"720p","aspect_ratio":"16:9"}`)}
		if e := s.database.CreateRenderJob(j); e != nil {
			t.Fatal(e)
		}
	}
	job("succeeded")
	job("failed")                  // the service does not charge for a failed clip
	job("reconciliation_required") // unknown: shown separately, never in the total
	asset := func(kind, path, meta string) string {
		a := &models.Asset{ProjectID: &project, Type: kind, Name: kind, FilePath: path, TagsJSON: []byte("[]"), MetadataJSON: []byte(meta)}
		if e := s.database.CreateAsset(a); e != nil {
			t.Fatal(e)
		}
		return a.ID
	}
	picture := asset("storyboard", "storage/assets/a.png", `{"provider":"openai","model":"gpt-image-2","scene_id":"`+scene.ID+`","usage":{"input_tokens":100,"output_tokens":5000,"input_tokens_details":{"text_tokens":100,"image_tokens":0}}}`)
	asset("character_reference", "storage/assets/b.png", `{"provider":"codex","model":"native-image-generation"}`)
	asset("voiceover", "storage/voiceovers/c.mp3", `{"provider":"fish","model":"s2.1-pro","text":"Hello there, this is one spoken line."}`)
	asset("location", "storage/assets/d.png", `{}`) // an upload: nothing was paid

	read := func() map[string]any {
		w := call(t, s, "GET", "/costs", "")
		if w.Code != 200 {
			t.Fatalf("costs: %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	out := read()
	// 10 s of 720p ≈ $2.42, one metered picture = $0.1505, one voice line < 1 cent.
	if total := out["total"].(float64); total < 2.56 || total > 2.58 {
		t.Fatalf("total = %v, want about 2.57: %v", total, out)
	}
	if n := len(out["unconfirmed"].([]any)); n != 1 {
		t.Fatalf("unconfirmed = %d, want 1", n)
	}
	if n := len(out["categories"].([]any)); n != 3 {
		t.Fatalf("categories = %v", out["categories"])
	}
	episodes := out["episodes"].([]any)
	if first := episodes[0].(map[string]any); first["label"] != "First Noodles" || first["amount"].(float64) < 2.5 {
		t.Fatalf("episode row = %v", first)
	}
	if free := out["included"].([]any); len(free) != 1 {
		t.Fatalf("included = %v", free)
	}

	// Deleting what was paid for must not erase the spend, and re-reading must not double it.
	if e := s.database.DeleteAsset(picture); e != nil {
		t.Fatal(e)
	}
	if e := s.database.DeleteScene(scene.ID); e != nil {
		t.Fatal(e)
	}
	if again := read()["total"].(float64); again != out["total"].(float64) {
		t.Fatalf("total changed after deletion: %v → %v", out["total"], again)
	}
}
