package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

// referenceScene is a scene whose video service takes reference pictures: Chen Wei (with
// a reference sheet) is in the first shot, The Visitor (also with one) in none, and a
// rooftop picture is connected to the scene.
func referenceScene(t *testing.T, localText bool) (*Server, *models.Scene) {
	t.Helper()
	fastAutopilot(t)
	s, _ := falServer(t, "done")
	s.cfg.UseLocalText = localText
	argo := "https://argolink.io/v1"
	if e := s.database.SetProviderSecret("argolink", "test-key", &argo); e != nil {
		t.Fatal(e)
	}
	s.database.SetAppSetting("default_video_provider", `"argolink"`)
	s.database.SetAppSetting("video_model", `"seedance-2.0"`)
	project, _ := s.database.GetActiveProjectID()
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	picture := func(name, kind string, character *string) *models.Asset {
		file := "storage/" + strings.ReplaceAll(name, " ", "-") + ".png"
		_ = os.WriteFile(filepath.Join(s.cfg.StorageRoot, filepath.Base(file)), png, 0600)
		asset := &models.Asset{ProjectID: &project, Name: name, Type: kind, CharacterID: character, FilePath: file, MetadataJSON: []byte(`{}`)}
		if e := s.database.CreateAsset(asset); e != nil {
			t.Fatal(e)
		}
		return asset
	}
	cast := []string{}
	for _, name := range []string{"Chen Wei", "The Visitor"} {
		c := &models.Character{ProjectID: &project, Name: name, Appearance: name + " in plain clothes"}
		if e := s.database.CreateCharacter(c); e != nil {
			t.Fatal(e)
		}
		sheet := picture(name+" reference", "character_reference", &c.ID)
		c.ReferenceAssetIDsJSON = voiceJSON([]string{sheet.ID})
		if e := s.database.UpdateCharacter(c); e != nil {
			t.Fatal(e)
		}
		cast = append(cast, c.ID)
	}
	roof := picture("Dusk City Rooftop", "location", nil)
	scene := renderableScene(t, s, nil, "Rooftop", 1, "auto")
	shots, _ := s.database.ListShots(scene.ID)
	shots[0].Prompt = "Chen Wei leans on the railing at dusk"
	if e := s.database.UpdateShot(&shots[0]); e != nil {
		t.Fatal(e)
	}
	scene, _ = s.database.GetScene(scene.ID)
	var bound []string
	_ = json.Unmarshal(scene.AssetIDsJSON, &bound)
	scene.AssetIDsJSON = voiceJSON(append(bound, roof.ID))
	scene.CharacterIDsJSON = voiceJSON(cast)
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	scene, _ = s.database.GetScene(scene.ID)
	return s, scene
}

// Like Seedance's own canvas, the pictures connected to a scene go to the video model as
// references, named @Image 1, @Image 2 in the prompt, with no storyboard picture made.
func TestSceneClipsAreMadeFromTheirConnectedPictures(t *testing.T) {
	for _, localText := range []bool{false, true} {
		t.Run(fmt.Sprintf("local_text=%t", localText), func(t *testing.T) {
			s, scene := referenceScene(t, localText)
			project := *scene.ProjectID
			var plan productionPlan
			if w := call(t, s, "GET", "/scenes/"+scene.ID+"/production", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
				t.Fatal(w.Code, w.Body.String())
			}
			if plan.VideoInput != inputReferences || len(plan.Pictures) != 3 || plan.Pictures[0].Name != "Chen Wei" || !plan.Pictures[0].Sheet || plan.Pictures[2].Kind != "place" {
				t.Fatalf("plan pictures: %s %+v", plan.VideoInput, plan.Pictures)
			}
			if spend := productionSpend(plan, false); spend.Storyboards != 0 {
				t.Fatalf("reference mode still pays for a storyboard: %+v", spend)
			}
			var preview struct {
				Clips []struct {
					Prompt   string
					Pictures []referencePicture
				}
			}
			if w := call(t, s, "GET", "/scenes/"+scene.ID+"/video-prompt", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || len(preview.Clips) != 1 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, scene.ID)); w.Code != 202 {
				t.Fatal(w.Code, w.Body.String())
			}
			<-finishPendingVideos(t, s, project, 1)
			jobs, _ := s.database.ListRenderJobs(project)
			var request struct {
				Final    string             `json:"final_prompt"`
				Images   []string           `json:"image_urls"`
				Pictures []referencePicture `json:"reference_pictures"`
				Frames   any                `json:"frame_images"`
			}
			if len(jobs) != 1 || json.Unmarshal(jobs[0].RequestJSON, &request) != nil {
				t.Fatalf("jobs: %d", len(jobs))
			}
			// The Visitor is in no shot of this clip, so their picture is not sent.
			if len(request.Images) != 2 || request.Frames != nil || len(request.Pictures) != 2 || request.Pictures[0].Name != "Chen Wei" || request.Pictures[1].Name != "Dusk City Rooftop" {
				t.Fatalf("request pictures: %v %+v frames=%v", request.Images, request.Pictures, request.Frames)
			}
			for _, want := range []string{"Define the person in @Image 1 as Chen Wei", "@Image 2 is the place Dusk City Rooftop"} {
				if !strings.Contains(request.Final, want) {
					t.Errorf("prompt lacks %q:\n%s", want, request.Final)
				}
			}
			if strings.Contains(request.Final, "@Image 3") || request.Final != preview.Clips[0].Prompt || len(preview.Clips[0].Pictures) != 2 {
				t.Errorf("preview and request differ, or a third picture is named:\n%s\n---\n%s", request.Final, preview.Clips[0].Prompt)
			}
		})
	}
}

// The owner can keep a scene on first-frame mode, and a scene whose clips were already
// made from a first frame keeps them instead of paying for new ones.
func TestFirstFrameModeStaysWhereItIsChosenOrAlreadyUsed(t *testing.T) {
	s, scene := referenceScene(t, false)
	project := *scene.ProjectID
	ctx := t.Context()
	settings := sceneProduction(scene)
	settings.VideoInput = inputFirstFrame
	var doc map[string]any
	_ = json.Unmarshal(scene.SceneJSON, &doc)
	doc["production"] = settings
	scene.SceneJSON = voiceJSON(doc)
	frames, err := s.sceneProductionPlan(ctx, scene)
	if err != nil || frames.VideoInput != inputFirstFrame || len(frames.Pictures) != 0 {
		t.Fatalf("chosen first frame: %v %s", err, frames.VideoInput)
	}
	// A clip made from a first frame before reference mode existed.
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "argolink", Model: "seedance-2.0", Status: "succeeded", RequestJSON: voiceJSON(map[string]any{"render_unit": renderUnit{Key: frames.ID + ":0"}})}
	if e := s.database.CreateRenderJob(job); e != nil {
		t.Fatal(e)
	}
	settings.VideoInput = ""
	doc["production"] = settings
	scene.SceneJSON = voiceJSON(doc)
	kept, err := s.sceneProductionPlan(ctx, scene)
	if err != nil || kept.VideoInput != inputFirstFrame || kept.ID != frames.ID || kept.Reused != 1 {
		t.Fatalf("finished first-frame clip not kept: %v %s reused=%d", err, kept.VideoInput, kept.Reused)
	}
	settings.VideoInput = inputReferences
	doc["production"] = settings
	scene.SceneJSON = voiceJSON(doc)
	chosen, err := s.sceneProductionPlan(ctx, scene)
	if err != nil || chosen.VideoInput != inputReferences || chosen.Reused != 0 {
		t.Fatalf("asked-for references: %v %s", err, chosen.VideoInput)
	}
}
