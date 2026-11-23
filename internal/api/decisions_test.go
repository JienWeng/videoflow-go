package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/db"
)

func listedDecisions(t *testing.T, s *Server) map[string]db.ProjectDecision {
	t.Helper()
	w := call(t, s, "GET", "/decisions", "")
	var out struct {
		Decisions []db.ProjectDecision `json:"decisions"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	byKey := map[string]db.ProjectDecision{}
	for _, d := range out.Decisions {
		byKey[d.Key] = d
	}
	return byKey
}

func TestDecisionEndpoints(t *testing.T) {
	s := routerServer(t, nil)
	if got := listedDecisions(t, s); len(got) != 0 {
		t.Fatalf("new project: %v", got)
	}
	rejected := "rejected:render_scene:scene_1a2b"
	for _, tc := range []struct {
		key, body string
		code      int
	}{
		{"aspect_ratio", `{"value":" 9:16 "}`, 200},
		{url.PathEscape(rejected), `{"value":"Make video for the opening"}`, 200},
		{strings.ReplaceAll(rejected, ":", "%3A"), `{"value":"again"}`, 200},
		{"Bad%20Key", `{"value":"x"}`, 400},
		{"music", `{"value":"  "}`, 400},
		{"music", `{"value":"piano","source":"assistant"}`, 400},
		{"music", `{"value":"` + strings.Repeat("a", 501) + `"}`, 400},
	} {
		if w := call(t, s, "PUT", "/decisions/"+tc.key, tc.body); w.Code != tc.code {
			t.Fatalf("PUT %s %s: %d %s", tc.key, tc.body, w.Code, w.Body.String())
		}
	}
	got := listedDecisions(t, s)
	if d := got["aspect_ratio"]; d.Value != "9:16" || d.Source != db.DecisionByUser || len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[rejected].Value != "again" {
		t.Fatalf("%+v", got[rejected])
	}
	if w := call(t, s, "DELETE", "/decisions/"+strings.ReplaceAll(rejected, ":", "%3A"), ""); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(t, s, "DELETE", "/decisions/not-a-key", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if got = listedDecisions(t, s); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestRememberIsValidatedAndNeverOverrulesTheUser(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	s.recordDecision(project, "music", "no music")
	saved := s.rememberDecisions(project, []agents.RememberItem{
		{Key: "music", Value: "upbeat ukulele"},                // the user decided: kept as is
		{Key: "rejected:render_scene:s1", Value: "x"},          // only the user rejects
		{Key: "voice:char_1", Value: strings.Repeat("v", 400)}, // cut to size
		{Key: "notes", Value: "keeps it under a minute"},       // past the cap of three
	})
	if len(saved) != 1 || saved[0].Key != "voice:char_1" || len([]rune(saved[0].Value)) != maxRememberValueChars {
		t.Fatalf("saved %+v", saved)
	}
	if saved := s.rememberDecisions(project, []agents.RememberItem{{Key: "language", Value: "  "}, {Key: "Language", Value: "en"}}); len(saved) != 0 {
		t.Fatalf("saved %+v", saved)
	}
	got := listedDecisions(t, s)
	if got["music"].Value != "no music" || got["music"].Source != db.DecisionByUser || got["voice:char_1"].Source != db.DecisionByAssistant {
		t.Fatalf("%+v", got)
	}
	if _, kept := got["notes"]; kept || len(got) != 2 {
		t.Fatalf("remember is not capped: %+v", got)
	}
	for key, want := range map[string]bool{"language": true, "voice:c1": true, "notes:x": true, "rejected:a": false, "choice:x": false, "unknown_key": false, "": false} {
		if assistantMayRemember(key) != want {
			t.Errorf("assistantMayRemember(%q) = %v", key, !want)
		}
	}
}

func TestChatReadsAndWritesTheDecisionLog(t *testing.T) {
	answer := `{"reply":"Making the videos now.","action":"render_scenes","remember":[{"key":"music","value":"slow piano"}],"decision":{"mode":"act","assumptions":["slow piano"]}}`
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	storyScenes(t, s, "Opening", "Ending")
	project, _ := s.database.GetActiveProjectID()
	send := func(body string) (map[string]any, string) {
		t.Helper()
		w := call(t, s, "POST", "/chat", body)
		var out struct {
			Intent map[string]any `json:"intent"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		_, raw, _ := strings.Cut(*prompt, "Project context: ")
		return out.Intent, raw
	}

	// A quick reply is kept before planning, so the same turn already sees it.
	intent, raw := send(`{"message":"9:16","answer":{"key":"aspect_ratio","question":"Which format?","label":"Vertical","value":"9:16"}}`)
	var context struct {
		Decided struct {
			Rule      string              `json:"rule"`
			Decisions []map[string]string `json:"decisions"`
		} `json:"decided_so_far"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(context.Decided.Rule, "Decided so far") || len(context.Decided.Decisions) != 1 || context.Decided.Decisions[0]["value"] != "Vertical — 9:16" || context.Decided.Decisions[0]["by"] != "user" {
		t.Fatalf("decided_so_far: %+v", context.Decided)
	}
	if remembered, _ := intent["remembered"].([]any); len(remembered) != 1 {
		t.Fatalf("remembered: %v", intent["remembered"])
	}
	got := listedDecisions(t, s)
	if got["music"].Value != "slow piano" || got["music"].Source != "assistant" {
		t.Fatalf("%+v", got)
	}

	// A quick reply without a key is kept under its question.
	send(`{"message":"the first one","answer":{"question":"Which scene should open the story?","label":"The first one"}}`)
	if d, ok := listedDecisions(t, s)["choice:which_scene_should_open_the_story"]; !ok || d.Value != "The first one" {
		t.Fatalf("%+v", listedDecisions(t, s))
	}

	// A question the log already answers is overruled when there is a step to take…
	answer = `{"reply":"Making the videos.","action":"render_scenes","decision":{"mode":"ask","question":"Which format?","key":"aspect_ratio","reason":"options","options":[{"label":"Wide"}]}}`
	intent, _ = send(`{"message":"make the videos"}`)
	decision, _ := json.Marshal(intent["decision"])
	var d chatDecision
	json.Unmarshal(decision, &d)
	if d.Mode != decisionAct || d.Overruled != "already_decided" || !strings.Contains(d.Assumptions[0], "aspect_ratio = Vertical") {
		t.Fatalf("settled question: %+v", d)
	}
	// …but stands when the user is changing it.
	intent, _ = send(`{"message":"let's change the format"}`)
	decision, _ = json.Marshal(intent["decision"])
	json.Unmarshal(decision, &d)
	if d.Mode != decisionAsk {
		t.Fatalf("changing a choice: %+v", d)
	}
	if n, _ := s.database.ListDecisions(project); len(n) != 3 {
		t.Fatalf("log: %+v", n)
	}
}

// Confirming a brief keeps its choices and finishes the brief stage.
func TestUserActionsAreKeptInTheDecisionLog(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	s.recordBriefDecisions(project, generationBrief{Language: "es", AspectRatio: "9:16", PresetID: "soft-3d-animation", Platform: "TikTok", TargetDuration: 20, ConversationMode: "narration"})
	got := listedDecisions(t, s)
	for key, want := range map[string]string{"language": "es", "aspect_ratio": "9:16", "style_id": "soft-3d-animation", "platform": "TikTok", "target_duration": "20 seconds", "conversation_mode": "narration"} {
		if got[key].Value != want || got[key].Source != "user" {
			t.Errorf("%s = %+v", key, got[key])
		}
	}
	if _, kept := got["music"]; kept {
		t.Error("an empty choice was kept")
	}
	if m, _ := s.projectStages(t.Context(), project); m.Current != stageScript || stageByID(t, m, stageBrief).Status != statusDone {
		t.Fatalf("a confirmed brief does not finish the brief stage: %+v", m)
	}

}

// Saving an approved voice is kept; taking it back removes it. The speech service is
// never asked (the fake answers nothing and fails the test if called).
func TestSavedVoicesAreKeptInTheDecisionLog(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("saving a voice called a service") })
	s.cfg.GeminiAPIKey = "gemini-fixture"
	c := struct{ ID string }{voiceCharacter(t, s)}
	if r := call(t, s, "PUT", "/voice/preferences", `{"primary_provider":"gemini"}`); r.Code != 200 {
		t.Fatalf("choose gemini: %d %s", r.Code, r.Body.String())
	}
	if r := call(t, s, "PUT", "/voice/profiles/"+c.ID, `{"revision":0,"data":{"provider":"gemini","model":"auto","voice_id":"Kore","approved":true}}`); r.Code != 200 {
		t.Fatalf("save voice: %d %s", r.Code, r.Body.String())
	}
	if d := listedDecisions(t, s)["voice:"+c.ID]; d.Value != "gemini voice Kore" {
		t.Fatalf("voice: %+v", d)
	}
	if r := call(t, s, "PUT", "/voice/profiles/"+c.ID, `{"revision":1,"data":{"provider":"gemini","model":"auto","voice_id":"Kore","approved":false}}`); r.Code != 200 {
		t.Fatalf("unapprove voice: %d %s", r.Code, r.Body.String())
	}
	if _, kept := listedDecisions(t, s)["voice:"+c.ID]; kept {
		t.Fatal("a voice taken back is still kept")
	}
}
