package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestNameReplacerSwapsLongestFormsFirst(t *testing.T) {
	in := standIn{RealPerson: "Vitalik Buterin", NewName: "Leo Marsh", Replacements: []nameReplacement{
		{From: "Vitalik", To: "Leo"}, {From: "Vitalik Buterin", To: "Leo Marsh"}, {From: "维塔利克", To: "里奥"}, {From: "V", To: "x"},
	}}
	replace := nameReplacer("Vitalik Buterin", in)
	got := replace("Vitalik Buterin types. Vitalik sighs. 维塔利克说话。 Very calm.")
	if got != "Leo Marsh types. Leo sighs. 里奥说话。 Very calm." {
		t.Fatal(got)
	}
	raw := replaceJSON(json.RawMessage(`{"lines":[{"speaker":"Vitalik","text":"Vitalik here"}],"Vitalik":1}`), replace)
	if !strings.Contains(string(raw), `"speaker":"Leo"`) || !strings.Contains(string(raw), `"Vitalik":1`) {
		t.Fatal(string(raw))
	}
}

func TestApplyStandInRewritesStoryAndDropsPicture(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Vitalik Buterin", Description: "Creator of Ethereum", Appearance: "Tall, thin", ReferenceAssetIDsJSON: json.RawMessage(`["asset_x"]`)}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{ProjectID: &project, Title: "Vitalik at night", Summary: "Vitalik Buterin tells his father.", AspectRatio: "16:9", SceneJSON: json.RawMessage(`{"dialogue":[{"speaker":"Vitalik","line":"Hi"}]}`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	shot := &models.Shot{SceneID: scene.ID, Prompt: "Vitalik at the desk"}
	if err := s.database.CreateShot(shot); err != nil {
		t.Fatal(err)
	}
	in := standIn{CharacterID: c.ID, RealPerson: "Vitalik Buterin", NewName: "Leo Marsh", Appearance: "Stocky, curly dark hair", Replacements: []nameReplacement{{From: "Vitalik", To: "Leo"}}}
	if err := s.applyStandIn(project, c, in); err != nil {
		t.Fatal(err)
	}
	gotC, _ := s.database.GetCharacter(c.ID)
	gotScene, _ := s.database.GetScene(scene.ID)
	gotShot, _ := s.database.GetShot(shot.ID)
	if gotC.Name != "Leo Marsh" || gotC.Appearance != "Stocky, curly dark hair" || len(models.IDList(gotC.ReferenceAssetIDsJSON)) != 0 {
		t.Fatal(gotC.Name, gotC.Appearance, string(gotC.ReferenceAssetIDsJSON))
	}
	all := gotScene.Title + gotScene.Summary + string(gotScene.SceneJSON) + gotShot.Prompt
	if strings.Contains(all, "Vitalik") || !strings.Contains(gotScene.Summary, "Leo Marsh tells") || !strings.Contains(string(gotScene.SceneJSON), `"speaker":"Leo"`) {
		t.Fatal(all)
	}
}

func TestRefusedForSafety(t *testing.T) {
	for text, want := range map[string]bool{
		"Google Gemini could not make the video (failed): ... (code INVALID_ARGUMENT).": true,
		"blocked by safety filters":       true,
		"the service timed out":           false,
		"quota exceeded, try again later": false,
	} {
		if refusedForSafety(text) != want {
			t.Fatal(text)
		}
	}
}

func TestStandInsEndpointNeedsCharacters(t *testing.T) {
	s := routerServer(t, nil)
	if r := call(t, s, "POST", "/characters/stand-ins", `{}`); r.Code != 409 {
		t.Fatal(r.Code, r.Body.String())
	}
	if !selectedChatActionAllowed("character", standInAction) {
		t.Fatal("replace_real_people must work whatever is selected")
	}
}
