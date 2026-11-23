package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func spokenLinesBody(t *testing.T, scene *models.Scene, lines []map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"lines": lines, "expected_updated_at": scene.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// Editing what is said saves a revision, keeps the rest of the scene, and makes the
// scene's clips stale: the next plan no longer reuses a clip made from the old lines.
func TestSceneSpokenLinesSaveAndMakeClipsStale(t *testing.T) {
	s, scene := referenceScene(t)
	project := *scene.ProjectID
	cast := models.IDList(scene.CharacterIDsJSON)
	var doc map[string]any
	_ = json.Unmarshal(scene.SceneJSON, &doc)
	doc["spoken_lines"] = []SpokenLine{{ID: "s1_l1", CharacterID: cast[0], Language: "Chinese", Text: "如果钱不需要靠银行来信任呢？", Visible: true}}
	scene.SceneJSON = voiceJSON(doc)
	if err := s.database.UpdateScene(scene); err != nil {
		t.Fatal(err)
	}
	scene, _ = s.database.GetScene(scene.ID)
	before, err := s.sceneProductionPlan(t.Context(), scene)
	if err != nil {
		t.Fatal(err)
	}
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "argolink", Model: "seedance-2.0", Status: "succeeded", RequestJSON: voiceJSON(map[string]any{"render_unit": renderUnit{Key: before.Units[0].Key}})}
	if err = s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.sceneProductionPlan(t.Context(), scene); again.Reused != 1 {
		t.Fatalf("finished clip should be reused before the edit: %+v", again)
	}
	revisions, _ := s.database.ListRevisions("scene", scene.ID)

	path := "/scenes/" + scene.ID + "/spoken-lines"
	body := spokenLinesBody(t, scene, []map[string]any{
		{"id": "s1_l1", "character_id": cast[0], "language": "Chinese", "text": "  如果钱不靠银行呢？ ", "visible": true},
		{"id": "", "character_id": "", "language": "English", "text": "Narrator closes the scene.", "visible": true},
		{"id": "s1_l1", "character_id": cast[1], "language": "English", "text": "A repeated id.", "visible": true},
	})
	if w := call(t, s, "PUT", path, body); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	updated, _ := s.database.GetScene(scene.ID)
	lines := sceneSpokenLines(updated)
	if len(lines) != 3 || lines[2].ID == "s1_l1" || lines[2].ID == lines[1].ID || lines[0].ID != "s1_l1" || lines[0].Text != "如果钱不靠银行呢？" || !lines[0].Visible || lines[1].ID == "" || lines[1].ID == "s1_l1" || lines[1].CharacterID != "" || lines[1].Visible {
		t.Fatalf("saved lines: %+v", lines)
	}
	var kept map[string]json.RawMessage
	_ = json.Unmarshal(updated.SceneJSON, &kept)
	for key := range doc {
		if _, ok := kept[key]; !ok {
			t.Fatalf("scene metadata %q lost: %s", key, updated.SceneJSON)
		}
	}
	if after, _ := s.database.ListRevisions("scene", scene.ID); len(after) != len(revisions)+1 {
		t.Fatalf("revisions: %d then %d", len(revisions), len(after))
	}
	plan, err := s.sceneProductionPlan(t.Context(), updated)
	if err != nil || plan.ID == before.ID || plan.Reused != 0 || plan.Calls != len(plan.Units) {
		t.Fatalf("plan after edit still reuses old clip: %v %+v", err, plan)
	}
	if len(plan.Lines) != 3 || plan.Lines[0].Text != "如果钱不靠银行呢？" {
		t.Fatalf("plan lines: %+v", plan.Lines)
	}

	// The same expected time is now stale.
	if w := call(t, s, "PUT", path, body); w.Code != 409 {
		t.Fatalf("stale save: %d", w.Code)
	}
	// Clearing every line makes the scene silent.
	if w := call(t, s, "PUT", path, spokenLinesBody(t, updated, []map[string]any{})); w.Code != 200 {
		t.Fatalf("clear: %d %s", w.Code, w.Body.String())
	}
	cleared, _ := s.database.GetScene(scene.ID)
	if len(sceneSpokenLines(cleared)) != 0 || !strings.Contains(string(cleared.SceneJSON), `"spoken_lines":[]`) {
		t.Fatalf("cleared: %s", cleared.SceneJSON)
	}
}

func TestSceneSpokenLinesRejectInvalidLines(t *testing.T) {
	s, scene := referenceScene(t)
	cast := models.IDList(scene.CharacterIDsJSON)
	other, err := s.database.CreateProject("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	stranger := &models.Character{ProjectID: &other.ID, Name: "Stranger"}
	if err = s.database.CreateCharacter(stranger); err != nil {
		t.Fatal(err)
	}
	path := "/scenes/" + scene.ID + "/spoken-lines"
	line := func(id, character, language, text string) map[string]any {
		return map[string]any{"id": id, "character_id": character, "language": language, "text": text, "visible": true}
	}
	for name, lines := range map[string][]map[string]any{
		"empty text":      {line("a", cast[0], "English", "   ")},
		"long text":       {line("a", cast[0], "English", strings.Repeat("话", maxSpokenLineRunes+1))},
		"no language":     {line("a", cast[0], " ", "Hello")},
		"other project":   {line("a", stranger.ID, "English", "Hello")},
		"unknown speaker": {line("a", "char_missing", "English", "Hello")},
	} {
		if w := call(t, s, "PUT", path, spokenLinesBody(t, scene, lines)); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	for name, body := range map[string]string{
		"missing lines":  `{"expected_updated_at":"` + scene.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + `"}`,
		"unknown fields": `{"lines":[],"scene_json":{}}`,
	} {
		if w := call(t, s, "PUT", path, body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	unchanged, _ := s.database.GetScene(scene.ID)
	if !unchanged.UpdatedAt.Equal(scene.UpdatedAt) {
		t.Fatal("a rejected edit changed the scene")
	}
	if revisions, _ := s.database.ListRevisions("scene", scene.ID); len(revisions) != 0 {
		t.Fatalf("a rejected edit saved %d revisions", len(revisions))
	}
	if w := call(t, s, "PUT", "/scenes/missing/spoken-lines", spokenLinesBody(t, scene, nil)); w.Code != 404 {
		t.Fatalf("missing scene: %d", w.Code)
	}
}
