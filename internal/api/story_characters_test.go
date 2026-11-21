package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

func TestStorySceneBriefContainsOnlyItsOwnActorsAndReferences(t *testing.T) {
	s, project, _, brief := storyCharacterFixture(t, false, `{}`)
	a := &models.Character{ProjectID: &project, Name: "First", Appearance: "blue dog"}
	s.database.CreateCharacter(a)
	b := &models.Character{ProjectID: &project, Name: "Second", Appearance: "red dog"}
	s.database.CreateCharacter(b)
	refA := &models.Asset{ProjectID: &project, CharacterID: &a.ID, Name: "First photo", Type: "character_reference"}
	s.database.CreateAsset(refA)
	refB := &models.Asset{ProjectID: &project, CharacterID: &b.ID, Name: "Second photo", Type: "character_reference"}
	s.database.CreateAsset(refB)
	a.ReferenceAssetIDsJSON, _ = json.Marshal([]string{refA.ID})
	s.database.UpdateCharacter(a)
	b.ReferenceAssetIDsJSON, _ = json.Marshal([]string{refB.ID})
	s.database.UpdateCharacter(b)
	brief.CharacterIDs = []string{a.ID, b.ID}
	brief.CharacterNotes = "wrong aggregate notes"
	brief.ReferenceAssetIDs = []string{refA.ID, refB.ID}
	draft := &agents.ScriptDraft{Title: "Two scenes", Scenes: []agents.SceneDraft{{Title: "First scene", Summary: "First", Duration: 1, CharacterIDs: []string{a.ID}}, {Title: "Second scene", Summary: "Second", Duration: 1, CharacterIDs: []string{b.ID}}}}
	_, scenes, err := s.persistDraft(project, brief, draft)
	if err != nil {
		t.Fatal(err)
	}
	first := sceneBrief(&scenes[0])
	second := sceneBrief(&scenes[1])
	if !strings.Contains(first.CharacterNotes, "blue dog") || strings.Contains(first.CharacterNotes, "red dog") || len(first.ReferenceAssetIDs) != 1 || first.ReferenceAssetIDs[0] != refA.ID {
		t.Fatal("first scene inherited other identity")
	}
	if !strings.Contains(second.CharacterNotes, "red dog") || strings.Contains(second.CharacterNotes, "blue dog") || len(second.ReferenceAssetIDs) != 1 || second.ReferenceAssetIDs[0] != refB.ID {
		t.Fatal("second scene inherited other identity")
	}
}

func TestGuidedRenderScopeAvoidsStaleCastButKeepsExcludedReferenceGuard(t *testing.T) {
	s, project, _, _ := storyCharacterFixture(t, false, `{}`)
	s.cfg.UseLocalText = true
	asset := &models.Asset{ProjectID: &project, Type: "storyboard", MetadataJSON: []byte(`{"consistency_review":"rejected"}`)}
	s.database.CreateAsset(asset)
	refs, _ := json.Marshal([]string{asset.ID})
	scene := &models.Scene{ProjectID: &project, Title: "Scene", CharacterIDsJSON: []byte(`[]`), AssetIDsJSON: refs}
	s.database.CreateScene(scene)
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{CharacterIDs: []string{"deleted-character"}}})
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, RequestJSON: []byte(`{}`)}
	if err := s.freezeDirectorVideo(ctx, job); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatal("scene scope must ignore incidental selection and retain approval", err)
	}

	if err := s.freezeDirectorVideo(guidedStoryContext(ctx, project, nil), job); err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatal("guided rendering lost its approval gate", err)
	}
}

const dogStoryDraft = `{"title":"Home","summary":"A puppy finds his mother","characters":[{"key":"pup","name":"Pip","description":"A lost puppy","appearance":"small golden puppy","personality":"curious"},{"key":"mother","name":"Mother dog","description":"Pip's mother","appearance":"large cream dog","personality":"gentle"}],"scenes":[{"title":"Reunion","summary":"Pip finds his mother","duration":1,"aspect_ratio":"16:9","character_ids":["new:pup","new:mother"],"spoken_lines":[{"id":"line-1","character_id":"new:pup","language":"English","text":"Mama!","visible":true}]}]}`

func storyCharacterFixture(t *testing.T, allow bool, raw string) (*Server, string, *agents.ScriptDraft, generationBrief) {
	t.Helper()
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call") })
	project, _ := s.database.GetActiveProjectID()
	var draft agents.ScriptDraft
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		t.Fatal(err)
	}
	var brief generationBrief
	json.Unmarshal([]byte(fmt.Sprintf(`{"idea":"A lost puppy finds his mother","scene_count":1,"aspect_ratio":"16:9","create_characters":%t}`, allow)), &brief)
	return s, project, &draft, brief
}

func TestStoryCharacterPlanningReusesOnlySelectedIdentity(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			s, project, draft, brief := storyCharacterFixture(t, true, dogStoryDraft)
			old := &models.Character{ProjectID: &project, Name: "Pip", Appearance: "approved black puppy"}
			s.database.CreateCharacter(old)
			if selected {
				brief.CharacterIDs = []string{old.ID}
			}
			script, scenes, err := s.persistDraft(project, brief, draft)
			if !selected {
				if err == nil {
					t.Fatal("silently adopted an unselected existing identity")
				}
				chars, _ := s.database.ListCharacters(project)
				if len(chars) != 1 {
					t.Fatal("failed plan left new characters")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(script.DraftJSON), "small golden puppy") {
				t.Fatal("story snapshot contradicts approved identity")
			}
			saved, _ := s.database.GetCharacter(old.ID)
			if saved.Appearance != old.Appearance {
				t.Fatal("existing identity overwritten")
			}
			if !strings.Contains(string(scenes[0].CharacterIDsJSON), old.ID) {
				t.Fatal("selected identity not bound")
			}
			chars, _ := s.database.ListCharacters(project)
			if len(chars) != 2 {
				t.Fatal("selected identity was duplicated")
			}
		})
	}
}

func TestStoryCharacterProfilesAndSpeakerBindingsAreSavedTogether(t *testing.T) {
	s, project, draft, brief := storyCharacterFixture(t, true, dogStoryDraft)
	_, scenes, err := s.persistDraft(project, brief, draft)
	if err != nil {
		t.Fatal(err)
	}
	chars, _ := s.database.ListCharacters(project)
	if len(chars) != 2 {
		t.Fatalf("expected two character profiles, got %d", len(chars))
	}
	var cast []string
	json.Unmarshal(scenes[0].CharacterIDsJSON, &cast)
	if len(cast) != 2 {
		t.Fatalf("cast missing: %v", cast)
	}
	for _, id := range cast {
		if _, e := s.database.GetCharacter(id); e != nil {
			t.Fatal(e)
		}
	}
	if !strings.Contains(sceneBrief(&scenes[0]).CharacterNotes, "small golden puppy") {
		t.Fatal("saved scene lost new appearance")
	}
	var data struct {
		SpokenLines []agents.SpokenLine `json:"spoken_lines"`
	}
	json.Unmarshal(scenes[0].SceneJSON, &data)
	if len(data.SpokenLines) != 1 || data.SpokenLines[0].CharacterID != cast[0] {
		t.Fatalf("speaker not remapped: %+v", data.SpokenLines)
	}
}

func TestStoryCharacterPlanningMustBeExplicitAndRejectUnknownBindings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		allow bool
		raw   string
	}{
		{"opt-out", false, dogStoryDraft},
		{"unknown-speaker", true, strings.Replace(dogStoryDraft, `"character_id":"new:pup"`, `"character_id":"char_foreign"`, 1)},
		{"unknown-scene-cast", true, strings.Replace(dogStoryDraft, `"new:pup","new:mother"`, `"new:missing","new:mother"`, 1)},
		{"duplicate-key", true, strings.Replace(dogStoryDraft, `"key":"mother"`, `"key":"pup"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, project, draft, brief := storyCharacterFixture(t, tc.allow, tc.raw)
			if _, _, err := s.persistDraft(project, brief, draft); err == nil {
				t.Fatal("invalid character plan accepted")
			}
			chars, _ := s.database.ListCharacters(project)
			scripts, _ := s.database.ListScripts(project)
			scenes, _ := s.database.ListScenes(project)
			if len(chars)+len(scripts)+len(scenes) != 0 {
				t.Fatal("invalid draft left partial records")
			}
		})
	}
}

type storyPipelineTestOptions struct{ fractional, replaceReference, failReference, reuseProp bool }

func runGuidedStoryCharacterPipeline(t *testing.T, imageFailure bool, options ...storyPipelineTestOptions) {
	opts := storyPipelineTestOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	var storyboardPayload atomic.Value
	replacementURL := ""
	var images, videos atomic.Int32
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"input_references":{"type":"array"},"aspect_ratio":{"type":"enum","values":["16:9","1:1"]}}}]}`))
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1,4],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		case "/images":
			images.Add(1)
			if (imageFailure && images.Load() == 3) || (opts.failReference && images.Load() == 2) {
				http.Error(w, "fixture image failure", 500)
				return
			}
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			refs, _ := payload["input_references"].([]any)
			if images.Load() <= 2 && len(refs) != 0 {
				t.Error("character sheet inherited another actor's references")
			}
			if images.Load() >= 3 {
				expected := 2
				if opts.reuseProp {
					expected = 3
				}
				if len(refs) != expected {
					t.Error("storyboard lost cast/prop references", payload)
				}
				raw, _ := json.Marshal(payload)
				storyboardPayload.Store(string(raw))
			}
			data := []byte{137, 80, 78, 71, 13, 10, 26, 10, byte(images.Load())}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(data), "media_type": "image/png"}}})
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
	var prop *models.Asset
	if opts.reuseProp {
		os.WriteFile(filepath.Join(s.cfg.StorageRoot, "collar.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
		prop = &models.Asset{ProjectID: &project, Type: "prop", Name: "Red collar", Description: "Red leather collar", FilePath: "storage/collar.png", MetadataJSON: []byte(`{"consistency_review":"pending","locked_traits":{"color":"red"}}`)}
		s.database.CreateAsset(prop)
	}
	session, _ := s.database.CreateDirectorSession(project, "codex", "Dog story")
	s.database.SetDirectorSelection(session.ID, project, []byte(`{"character_ids":["char_deleted"],"asset_ids":[]}`))
	fake := filepath.Join(t.TempDir(), "codex")
	program := `#!/usr/bin/env python3
import sys,json
p=sys.stdin.read()
result={'prompt':'Pip, a small golden puppy, meets his cream-colored mother'}
if 'scriptwriter' in p: result=json.loads(` + fmt.Sprintf("%q", dogStoryDraft) + `)
elif 'Plan only essential visible props' in p: result={'assets':[]}
elif 'cinematographer' in p: result={'shots':[{'shot_order':1,'duration':1,'prompt':'Pip and mother','camera':'wide','movement':'still'}]}
if 'precise image-generation prompt' in p and 'Character reference sheet' not in p and 'small golden puppy' not in p: sys.exit(2)
print(json.dumps({'type':'thread.started','thread_id':'dog-native'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps(result)}}))
print(json.dumps({'type':'turn.completed'}))
`
	if opts.fractional {
		program = strings.Replace(program, "'duration':1,'prompt':'Pip and mother'", "'duration':1.2,'prompt':'Pip and mother'", 1)
		if !strings.Contains(program, "'duration':1.2") {
			t.Fatal("fractional fixture was not installed")
		}
	}
	if opts.reuseProp {
		plan, _ := json.Marshal(map[string]any{"assets": []any{map[string]any{"name": prop.Name, "type": "prop", "description": prop.Description, "existing_asset_id": prop.ID, "reason": "Existing red collar matches"}}})
		program = strings.Replace(program, "result={'assets':[]}", "result=json.loads("+fmt.Sprintf("%q", string(plan))+")", 1)
	}
	os.WriteFile(fake, []byte(program), 0700)
	s.directorRunner.Codex = fake
	w := call(t, s, "POST", "/videos/generate", `{"idea":"A puppy finds his mother","scene_count":1,"target_duration":1,"aspect_ratio":"16:9","create_characters":true,"character_ids":[]}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var start struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &start)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		op, _ := s.database.GetOp(start.ID)
		if op.Status == "failed" {
			if imageFailure || opts.failReference {
				var saved struct {
					Characters []string `json:"created_character_ids"`
					Scenes     []string `json:"scene_ids"`
					Script     string   `json:"script_id"`
				}
				json.Unmarshal(op.ResultJSON, &saved)
				chars, _ := s.database.ListCharacters(project)
				expectedImages := int32(3)
				if opts.failReference {
					expectedImages = 2
				}
				if opts.failReference {
					bound := 0
					for _, c := range chars {
						var ids []string
						json.Unmarshal(c.ReferenceAssetIDsJSON, &ids)
						bound += len(ids)
					}
					if bound != 1 {
						t.Fatal("earlier sheet lost after later failure", bound)
					}
				}
				if len(chars) != 2 || len(saved.Characters) != 2 || len(saved.Scenes) != 1 || saved.Script == "" || images.Load() != expectedImages || videos.Load() != 0 {
					t.Fatal("late failure lost saved work", string(op.ResultJSON))
				}
				return
			}
			t.Fatal("pipeline failed:", *op.Error)
		}
		if op.Status == "succeeded" {

			if imageFailure {
				t.Fatal("expected image-stage failure")
			}
			chars, _ := s.database.ListCharacters(project)
			if len(chars) != 2 || images.Load() != 3 || videos.Load() != 0 || !strings.Contains(string(op.ResultJSON), "ready_to_render") {
				t.Fatal("automatic story workflow lost", len(chars), images.Load(), videos.Load(), string(op.ResultJSON))
			}
			scenes, err := s.database.ListScenes(project)
			if err != nil {
				t.Fatal(err)
			}
			for _, scene := range scenes {
				shots, err := s.database.ListShots(scene.ID)
				if err != nil {
					t.Fatal(err)
				}
				total := 0
				for _, shot := range shots {
					if shot.Duration <= 0 {
						t.Fatal("nonpositive persisted shot")
					}
					total += shot.Duration
				}
				if total != scene.Duration {
					t.Fatalf("shot timing %d does not match scene %d", total, scene.Duration)
				}
			}
			ids := []string{}
			for _, c := range chars {
				ids = append(ids, c.ID)
			}
			reused, usable, reuseErr := s.storyCharacterReferences(guidedStoryContext(context.Background(), project, ids), project, ids, "same style")
			if reuseErr != nil || !usable || len(reused) != 2 || images.Load() != 3 {
				t.Fatal("references were not reused without review", reuseErr, images.Load())
			}
			if opts.replaceReference {
				// Previously paused operations remain resumable without image approval.
				c := &chars[0]
				path := filepath.Join(s.cfg.StorageRoot, "replacement.png")
				os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10, 99}, 0600)
				asset := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: "Replacement", FilePath: "storage/replacement.png", MetadataJSON: []byte(`{"consistency_review":"pending"}`)}
				s.database.CreateAsset(asset)
				replacementURL, _ = s.imageURL(asset)
				c.ReferenceAssetIDsJSON, _ = json.Marshal([]string{asset.ID})
				s.database.UpdateCharacter(c)
				var state storyContinuation
				json.Unmarshal(op.ResultJSON, &state)
				state.Status = "awaiting_character_review"
				raw, _ := json.Marshal(state)
				s.database.UpdateOp(op.ID, "succeeded", raw, nil)
				resumed := call(t, s, "POST", "/videos/generate/"+op.ID+"/continue", `{}`)
				if resumed.Code != 202 {
					t.Fatal(resumed.Code, resumed.Body.String())
				}
				duplicate := call(t, s, "POST", "/videos/generate/"+op.ID+"/continue", `{}`)
				if duplicate.Code != 409 {
					t.Fatal("duplicate continuation", duplicate.Code)
				}
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					next, _ := s.database.GetOp(op.ID)
					if next.Status == "succeeded" {
						break
					}
					if next.Status == "failed" {
						t.Fatal("legacy continuation failed", string(next.ResultJSON), next.Error)
					}
					time.Sleep(10 * time.Millisecond)
				}
				payload, _ := storyboardPayload.Load().(string)
				if images.Load() != 4 || !strings.Contains(payload, replacementURL) || videos.Load() != 0 {
					t.Fatal("legacy replacement not used", images.Load(), payload)
				}
			}

			selection, _ := s.database.GetDirectorSelection(session.ID)
			if !strings.Contains(string(selection), "char_deleted") {
				t.Fatal("guided creation rewrote saved session references")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pipeline did not finish")
}

func TestStoryExplicitEmptySceneCastStaysEmpty(t *testing.T) {
	var raw map[string]any
	json.Unmarshal([]byte(dogStoryDraft), &raw)
	scenes := raw["scenes"].([]any)
	raw["scenes"] = append([]any{map[string]any{"title": "Empty lane", "summary": "An empty lane", "duration": 1, "aspect_ratio": "16:9", "character_ids": []string{}, "spoken_lines": []any{}}}, scenes...)
	encoded, _ := json.Marshal(raw)
	s, project, draft, brief := storyCharacterFixture(t, true, string(encoded))
	_, saved, err := s.persistDraft(project, brief, draft)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved[0].CharacterIDsJSON) != "[]" || sceneBrief(&saved[0]).CharacterNotes != "" || len(sceneBrief(&saved[0]).ReferenceAssetIDs) != 0 {
		t.Fatal("empty scene inherited actors")
	}
}

func TestGuidedStoryIgnoresStaleSessionCastAndCreatesNeededCharacters(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, false)
}
func TestGuidedStoryLateImageFailureRetainsCreatedProfilesAndScenes(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, true)
}

func TestGuidedStoryAcceptsFractionalShotTimingWithoutRendering(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, false, storyPipelineTestOptions{fractional: true})
}

func TestGuidedStoryUsesCurrentReplacementReferences(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, false, storyPipelineTestOptions{replaceReference: true})
}
func TestGuidedStoryReferenceFailurePreservesEarlierSheet(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, false, storyPipelineTestOptions{failReference: true})
}

func TestGuidedStoryReusesPropAndRequiresItsApproval(t *testing.T) {
	runGuidedStoryCharacterPipeline(t, false, storyPipelineTestOptions{reuseProp: true})
}
