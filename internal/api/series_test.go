package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestNextEpisodeKeepsFormatCastAndStorySoFar(t *testing.T) {
	answer := `{"reply":"Next one!","action":"generate_script","next_episode":true,"creation_brief":{"idea":"The robot opens a noodle stall","aspect_ratio":"16:9","target_duration":60}}`
	s := routerServer(t, fakeChatModel(answer))
	project, _ := s.database.GetActiveProjectID()
	call(t, s, "POST", "/characters", `{"name":"Robo","appearance":"Small blue robot"}`)
	chars, _ := s.database.ListCharacters(project)
	script := &models.Script{ProjectID: &project, Idea: "robot cooks", Title: "First Noodles", Summary: "Robo burns the noodles, then gets it right.", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	cast, _ := json.Marshal([]string{chars[0].ID})
	if e := s.database.CreateScene(&models.Scene{ProjectID: &project, ScriptID: &script.ID, SceneOrder: 1, Title: "Kitchen", Duration: 10, AspectRatio: "9:16", CharacterIDsJSON: cast, SceneJSON: []byte("{}")}); e != nil {
		t.Fatal(e)
	}
	op := &models.Op{Kind: "video_generation", Status: "running", ProjectID: &project}
	if e := s.database.CreateOp(op); e != nil {
		t.Fatal(e)
	}
	result := `{"brief":{"platform":"TikTok","aspect_ratio":"9:16","resolution":"720p","language":"English","conversation_mode":"dialogue","target_duration":20,"scene_count":2,"tone":"Playful","audience":"Families","style_preset_id":"soft-3d-animation"}}`
	if e := s.database.UpdateOp(op.ID, "succeeded", json.RawMessage(result), nil); e != nil {
		t.Fatal(e)
	}

	w := call(t, s, "POST", "/chat", `{"message":"make the next episode"}`)
	var body struct {
		Intent struct {
			Reply   string
			CastIDs []string `json:"cast_ids"`
			Episode int      `json:"episode_number"`
			Brief   struct {
				Idea, Instruction, Platform, Tone string
				Aspect                            string `json:"aspect_ratio"`
				Duration                          int    `json:"target_duration"`
				Scenes                            int    `json:"scene_count"`
			} `json:"creation_brief"`
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	i := body.Intent
	if i.Episode != 2 || len(i.CastIDs) != 1 || i.CastIDs[0] != chars[0].ID || !strings.HasPrefix(i.Reply, "Episode 2") {
		t.Fatalf("%+v", i)
	}
	// The series format wins over what the planner guessed; the recap carries the story.
	if i.Brief.Aspect != "9:16" || i.Brief.Duration != 20 || i.Brief.Scenes != 2 || i.Brief.Platform != "TikTok" || i.Brief.Tone != "Playful" || !strings.Contains(i.Brief.Instruction, `Episode 1, "First Noodles"`) || i.Brief.Idea != "The robot opens a noodle stall" {
		t.Fatalf("%+v", i.Brief)
	}

	w = call(t, s, "GET", "/series", "")
	if !strings.Contains(w.Body.String(), `"number":1`) || !strings.Contains(w.Body.String(), "First Noodles") {
		t.Fatal(w.Body.String())
	}
}

func TestFinishVideoTargetsNewestEpisodeAndAutopilotChecksTheStory(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{"reply":"On it.","action":"finish_video"}`))
	project, _ := s.database.GetActiveProjectID()

	// No story yet: nothing to finish.
	w := call(t, s, "POST", "/chat", `{"message":"just make the whole video"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"action":"unknown"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	var last string
	for _, title := range []string{"One", "Two"} {
		script := &models.Script{ProjectID: &project, Idea: title, Title: title, Summary: title, DraftJSON: []byte("{}")}
		if e := s.database.CreateScript(script); e != nil {
			t.Fatal(e)
		}
		if e := s.database.CreateScene(&models.Scene{ProjectID: &project, ScriptID: &script.ID, SceneOrder: 1, Title: title + " scene", Duration: 5, AspectRatio: "9:16", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}); e != nil {
			t.Fatal(e)
		}
		last = script.ID
	}
	w = call(t, s, "POST", "/chat", `{"message":"just make the whole video","full":true,"auto":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"action":"finish_video"`) || !strings.Contains(w.Body.String(), `"script_id":"`+last+`"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(t, s, "POST", "/autopilot/finish", `{"script_id":"script_missing"}`); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A scene with no shots cannot be rendered: autopilot stops with a plain reason and spends nothing.
	if w = call(t, s, "POST", "/autopilot/finish", `{"script_id":"`+last+`"}`); w.Code != 502 || !strings.Contains(w.Body.String(), "isn't ready for video") {
		t.Fatal(w.Code, w.Body.String())
	}
}
