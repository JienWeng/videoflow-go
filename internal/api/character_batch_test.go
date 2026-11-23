package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"videoflow-go/internal/models"
)

// sheetService is a fake picture service with a fake local planner. Each picture call
// stays open until three are open (or a short wait passes), so a batch that drew more
// than three at once would be seen. Pictures of a character named in fail are refused.
type sheetService struct {
	fail    atomic.Value // string
	calls   atomic.Int32
	open    atomic.Int32
	mu      sync.Mutex
	maxOpen int32
}

var sheetName = regexp.MustCompile(`reference sheet of (.+?) with exactly`)

func (f *sheetService) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			// The local session writes the picture prompt; keep the character's name in it.
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			name := "someone"
			for _, m := range body.Messages {
				if found := sheetName.FindStringSubmatch(m.Content); found != nil {
					name = found[1]
				}
			}
			prompt, _ := json.Marshal(map[string]string{"prompt": "Sheet of " + name})
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(prompt)}}}})
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["1:1"]},"input_references":{"type":"array"}}}]}`))
		case "/images":
			f.calls.Add(1)
			open := f.open.Add(1)
			f.mu.Lock()
			f.maxOpen = max(f.maxOpen, open)
			f.mu.Unlock()
			for deadline := time.Now().Add(1500 * time.Millisecond); f.open.Load() < 3 && time.Now().Before(deadline); {
				time.Sleep(10 * time.Millisecond)
			}
			time.Sleep(50 * time.Millisecond)
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			f.open.Add(-1)
			if fail, _ := f.fail.Load().(string); fail != "" && strings.Contains(fmt.Sprint(payload["prompt"]), "Sheet of "+fail) {
				http.Error(w, "fixture picture refused", 500)
				return
			}
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		default:
			t.Errorf("unexpected service path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}
}

// characterWithSheet adds a character whose reference picture is ready to use.
func characterWithSheet(t *testing.T, s *Server, project, name string) *models.Character {
	t.Helper()
	c := &models.Character{ProjectID: &project, Name: name, Appearance: "drawn"}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(s.cfg.StorageRoot, 0700)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "sheet-"+c.ID+".png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	meta, _ := json.Marshal(map[string]string{"consistency_review": "approved"})
	a := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: name, FilePath: "storage/sheet-" + c.ID + ".png", MetadataJSON: meta}
	if err := s.database.CreateAsset(a); err != nil {
		t.Fatal(err)
	}
	c.ReferenceAssetIDsJSON, _ = json.Marshal([]string{a.ID})
	if err := s.database.UpdateCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBatchDrawsEveryCharacterWithoutAPictureThreeAtATime(t *testing.T) {
	f := &sheetService{}
	f.fail.Store("Bo")
	s := routerServer(t, f.handler(t))
	project, _ := s.database.GetActiveProjectID()
	drawn := characterWithSheet(t, s, project, "Already Drawn")
	before := string(drawn.ReferenceAssetIDsJSON)
	ids := map[string]string{}
	for _, name := range []string{"Ana", "Bo", "Cy", "Dee", "Eli"} {
		c := &models.Character{ProjectID: &project, Name: name, Appearance: "plain"}
		if err := s.database.CreateCharacter(c); err != nil {
			t.Fatal(err)
		}
		ids[name] = c.ID
	}

	// The plan is free and lists exactly the characters without a picture.
	var plan characterSheetPlan
	w := call(t, s, "GET", "/characters/reference-sheets", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(plan.Characters) != 5 || plan.ImageCalls != 5 || !plan.Ready || f.calls.Load() != 0 {
		t.Fatalf("plan = %+v, picture calls %d", plan, f.calls.Load())
	}
	for _, c := range plan.Characters {
		if c.CharacterID == drawn.ID {
			t.Fatal("a character that already has a picture was planned")
		}
	}

	// A list that changed since it was shown spends nothing.
	if w := call(t, s, "POST", "/characters/reference-sheets", `{"expected_image_calls":4}`); w.Code != 409 || f.calls.Load() != 0 {
		t.Fatalf("changed list accepted: %d %s", w.Code, w.Body.String())
	}

	w = call(t, s, "POST", "/characters/reference-sheets", `{"expected_image_calls":5}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var batch characterSheetBatch
	if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 5 {
		t.Fatalf("%d picture calls, want one per character", f.calls.Load())
	}
	f.mu.Lock()
	most := f.maxOpen
	f.mu.Unlock()
	if most > characterSheetConcurrency || most < 2 {
		t.Fatalf("%d pictures were drawn at once, want 2 to %d", most, characterSheetConcurrency)
	}
	// One refusal does not stop the others, and says which character and why.
	if len(batch.Made) != 4 || len(batch.Failed) != 1 || batch.Failed[0].CharacterID != ids["Bo"] || !strings.Contains(batch.Failed[0].Error, "fixture picture refused") {
		t.Fatalf("batch = %+v", batch)
	}
	for name, id := range ids {
		c, _ := s.database.GetCharacter(id)
		has := s.characterHasSheet(c)
		if has == (name == "Bo") {
			t.Fatalf("%s has a picture: %v", name, has)
		}
	}
	if c, _ := s.database.GetCharacter(drawn.ID); string(c.ReferenceAssetIDsJSON) != before {
		t.Fatal("a character that already had a picture was drawn again")
	}

	// Asking again draws only the one that is still missing.
	json.Unmarshal(call(t, s, "GET", "/characters/reference-sheets", "").Body.Bytes(), &plan)
	if len(plan.Characters) != 1 || plan.Characters[0].CharacterID != ids["Bo"] {
		t.Fatalf("second plan = %+v", plan)
	}
}

func TestBatchOfOnlyFailuresFailsAndAnEmptyBatchSpendsNothing(t *testing.T) {
	f := &sheetService{}
	f.fail.Store("Solo")
	s := routerServer(t, f.handler(t))
	project, _ := s.database.GetActiveProjectID()
	solo := &models.Character{ProjectID: &project, Name: "Solo"}
	s.database.CreateCharacter(solo)
	w := call(t, s, "POST", "/characters/reference-sheets", `{"character_ids":["`+solo.ID+`"]}`)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "Solo") {
		t.Fatalf("all-failed batch: %d %s", w.Code, w.Body.String())
	}
	f.fail.Store("")
	// A character from another project is refused before anything is spent.
	other, _ := s.database.CreateProject("Other", "")
	stranger := &models.Character{ProjectID: &other.ID, Name: "Stranger"}
	s.database.CreateCharacter(stranger)
	calls := f.calls.Load()
	if w := call(t, s, "POST", "/characters/reference-sheets", `{"character_ids":["`+stranger.ID+`"]}`); w.Code != 404 || f.calls.Load() != calls {
		t.Fatalf("other project's character accepted: %d", w.Code)
	}
	// Once every character has a picture there is nothing to make.
	call(t, s, "POST", "/characters/reference-sheets", `{}`)
	calls = f.calls.Load()
	if w := call(t, s, "POST", "/characters/reference-sheets", `{}`); w.Code != 409 || f.calls.Load() != calls {
		t.Fatalf("empty batch: %d %s", w.Code, w.Body.String())
	}
}

func TestBatchIsRefusedWithoutAPictureKey(t *testing.T) {
	f := &sheetService{}
	s := routerServer(t, f.handler(t))
	s.cfg.GeminiAPIKey = ""
	s.database.SetAppSetting("default_image_provider", `"gemini"`)
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateCharacter(&models.Character{ProjectID: &project, Name: "Pip"})

	var plan characterSheetPlan
	json.Unmarshal(call(t, s, "GET", "/characters/reference-sheets", "").Body.Bytes(), &plan)
	if plan.Ready || !strings.Contains(plan.Problem, "key") || len(plan.Characters) != 1 {
		t.Fatalf("plan without a key = %+v", plan)
	}
	w := call(t, s, "POST", "/characters/reference-sheets?background=true", `{}`)
	ops, _ := s.database.ListOps(project, 10)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "key") || f.calls.Load() != 0 || len(ops) != 0 {
		t.Fatalf("batch without a key: %d %s, %d calls, %d ops", w.Code, w.Body.String(), f.calls.Load(), len(ops))
	}
}

// Full auto asks the run to draw the missing cast pictures first; when they cannot be
// made the run stops before any video is paid for.
func TestFullAutoRunStopsBeforeVideosWhenCastPicturesCannotBeMade(t *testing.T) {
	fastAutopilot(t)
	s, fal := falServer(t, "done")
	s.cfg.GeminiAPIKey = ""
	s.database.SetAppSetting("default_image_provider", `"gemini"`)
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Idea: "pup", Title: "Pup", Summary: "pup", DraftJSON: []byte("{}")}
	s.database.CreateScript(script)
	scene := renderableScene(t, s, &script.ID, "Opening", 1, "auto")
	pip := &models.Character{ProjectID: &project, Name: "Pip"}
	s.database.CreateCharacter(pip)
	scene.CharacterIDsJSON, _ = json.Marshal([]string{pip.ID})
	s.database.UpdateScene(scene)

	w := call(t, s, "POST", "/autopilot/finish", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true,"character_pictures":true}`, scene.ID))
	jobs, _ := s.database.ListRenderJobs(project)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "character pictures") || fal.posts.Load() != 0 || len(jobs) != 0 {
		t.Fatalf("run went on without the cast pictures: %d %s, %d video requests, %d jobs", w.Code, w.Body.String(), fal.posts.Load(), len(jobs))
	}
}

func TestChatOffersOneButtonForEveryCharacterWithoutAPicture(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{"reply":"I'll draw them all.","action":"generate_character_references"}`))
	project, _ := s.database.GetActiveProjectID()
	drawn := characterWithSheet(t, s, project, "Drawn")
	a := &models.Character{ProjectID: &project, Name: "Ana"}
	b := &models.Character{ProjectID: &project, Name: "Bo"}
	s.database.CreateCharacter(a)
	s.database.CreateCharacter(b)
	var reply struct {
		Intent struct {
			Action       string   `json:"action"`
			CharacterIDs []string `json:"character_ids"`
			Characters   []struct {
				Name string `json:"name"`
			} `json:"characters"`
			ImageCalls int `json:"image_calls"`
		} `json:"intent"`
	}
	w := call(t, s, "POST", "/chat", `{"message":"make pictures for all my characters"}`)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reply) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	got := strings.Join(reply.Intent.CharacterIDs, ",")
	if reply.Intent.Action != "generate_character_references" || reply.Intent.ImageCalls != 2 || len(reply.Intent.Characters) != 2 || strings.Contains(got, drawn.ID) || !strings.Contains(got, a.ID) || !strings.Contains(got, b.ID) {
		t.Fatalf("intent = %+v", reply.Intent)
	}

	// With every character drawn there is no paid button.
	characterWithSheet(t, s, project, "Late")
	for _, c := range []*models.Character{a, b} {
		drawnAgain := characterWithSheet(t, s, project, c.Name+" copy")
		c.ReferenceAssetIDsJSON = drawnAgain.ReferenceAssetIDsJSON
		s.database.UpdateCharacter(c)
	}
	w = call(t, s, "POST", "/chat", `{"message":"make pictures for all my characters"}`)
	json.Unmarshal(w.Body.Bytes(), &reply)
	if reply.Intent.Action != "unknown" {
		t.Fatalf("offered a paid button with nothing to draw: %+v", reply.Intent)
	}
}
