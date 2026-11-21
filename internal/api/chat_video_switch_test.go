package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
)

const switchFamily = "seedance-2.0"

// brokenArgoLink makes ArgoLink the video service with a key it refused. OpenRouter keeps
// the key routerServer saved; extra lists other services to connect, with what each said.
func brokenArgoLink(t *testing.T, s *Server, extra map[string]providers.KeyCheck) {
	t.Helper()
	for name, check := range map[string]providers.KeyCheck{"argolink": providers.RejectedKey("ArgoLink")} {
		extra[name] = check
	}
	for name, check := range extra {
		key := "test-key-" + name
		if e := s.database.SetProviderSecret(name, key, nil); e != nil {
			t.Fatal(e)
		}
		if check.State != "" {
			if e := s.routes().SaveKeyCheck(name, key, check); e != nil {
				t.Fatal(e)
			}
		}
	}
	s.database.SetAppSetting("default_video_provider", `"argolink"`)
	s.database.SetAppSetting("video_model", `"`+costs.HostByName("argolink").Models[switchFamily]+`"`)
	if provider, _ := s.routes().MediaSelection("video"); provider != "argolink" {
		t.Fatalf("setup: videos use %s", provider)
	}
}

type switchChoice struct {
	Mode      string  `json:"mode"`
	Model     string  `json:"model"`
	Provider  string  `json:"provider"`
	Label     string  `json:"label"`
	PerSecond float64 `json:"per_second"`
}

func askSwitch(t *testing.T, s *Server, answer *string, switchTo string) (action, reply string, choice *switchChoice) {
	t.Helper()
	*answer = `{"reply":"ArgoLink refused the key. OpenRouter can make them now.","action":"switch_video_service","switch_to":"` + switchTo + `"}`
	w := call(t, s, "POST", "/chat", `{"message":"make the videos for both scenes","full":true,"auto":true}`)
	var out struct {
		Intent struct {
			Action string        `json:"action"`
			Reply  string        `json:"reply"`
			Switch *switchChoice `json:"video_switch"`
		} `json:"intent"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return out.Intent.Action, out.Intent.Reply, out.Intent.Switch
}

type switchTarget struct {
	ID        string  `json:"id"`
	Service   string  `json:"service"`
	PerSecond float64 `json:"dollars_per_second_720p"`
}

func plannerSwitchTargets(t *testing.T, prompt string) (problem string, targets []switchTarget) {
	t.Helper()
	_, raw, _ := strings.Cut(prompt, "Project context: ")
	var context struct {
		Service struct {
			Problem     string         `json:"problem"`
			CanSwitchTo []switchTarget `json:"can_switch_to"`
		} `json:"video_service"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatal(err)
	}
	return context.Service.Problem, context.Service.CanSwitchTo
}

func TestPlannerIsToldWhichVideoServicesCouldTakeOver(t *testing.T) {
	answer := `{"reply":"ok","action":"none"}`
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	brokenArgoLink(t, s, map[string]providers.KeyCheck{
		"byteplus": {},                              // connected, never checked: usable
		"fal":      providers.OutOfCredit("fal.ai"), // no balance left
	})

	sendChat(t, s, "make the videos for both scenes in episode 2")
	problem, targets := plannerSwitchTargets(t, *prompt)
	if !strings.Contains(problem, "ArgoLink did not accept this key") {
		t.Fatalf("problem = %q", problem)
	}
	ids := []string{}
	for i, target := range targets {
		ids = append(ids, target.ID)
		if target.PerSecond <= 0 || target.Service == "" || i > 0 && target.PerSecond < targets[i-1].PerSecond {
			t.Fatalf("targets not priced cheapest first: %+v", targets)
		}
	}
	if got := strings.Join(ids, ","); got != "byteplus,openrouter" && got != "openrouter,byteplus" {
		t.Fatalf("can_switch_to = %v; want only the usable byteplus and openrouter", ids)
	}
}

func TestChatOffersAVideoServiceSwitchThatOnlyTheRouteStores(t *testing.T) {
	answer := ""
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	brokenArgoLink(t, s, map[string]providers.KeyCheck{"fal": providers.OutOfCredit("fal.ai")})

	action, _, choice := askSwitch(t, s, &answer, "OpenRouter")
	_, targets := plannerSwitchTargets(t, *prompt)
	if action != "switch_video_service" || choice == nil || len(targets) != 1 {
		t.Fatalf("action = %s choice = %+v targets = %+v", action, choice, targets)
	}
	if *choice != (switchChoice{Mode: "manual", Model: switchFamily, Provider: "openrouter", Label: "OpenRouter", PerSecond: targets[0].PerSecond}) {
		t.Fatalf("card = %+v, want OpenRouter at the price the planner was told (%v)", *choice, targets[0].PerSecond)
	}
	// Even in full auto the reply changes nothing: the owner's click does.
	if provider, _ := s.routes().MediaSelection("video"); provider != "argolink" {
		t.Fatalf("the chat reply switched the service to %s by itself", provider)
	}
	body, _ := json.Marshal(map[string]string{"mode": choice.Mode, "model": choice.Model, "provider": choice.Provider})
	if w := call(t, s, "PUT", "/video-services/routing", string(body)); w.Code != 200 {
		t.Fatalf("click: %d %s", w.Code, w.Body.String())
	}
	if provider, model := s.routes().MediaSelection("video"); provider != "openrouter" || model != costs.HostByName("openrouter").Models[switchFamily] {
		t.Fatalf("after the click videos use %s %s", provider, model)
	}
	// Now in use, so there is nothing to switch to.
	if action, reply, _ := askSwitch(t, s, &answer, "openrouter"); action != "unknown" || !strings.Contains(reply, "already makes your videos") {
		t.Fatalf("switch to the service in use: %s %q", action, reply)
	}
}

func TestChatTurnsAnUnusableVideoServiceIntoAPlainReply(t *testing.T) {
	answer := ""
	s := routerServer(t, nil)
	plannerSays(s, &answer)
	brokenArgoLink(t, s, map[string]providers.KeyCheck{"fal": providers.OutOfCredit("fal.ai")})

	for want, says := range map[string]string{
		"argolink": "ArgoLink did not accept this key",
		"fal":      "no balance left",
		"byteplus": "Add its API key",
		"made-up":  "Add a working key",
		"kie":      "Add a working key", // a service the app no longer uses
	} {
		action, reply, choice := askSwitch(t, s, &answer, want)
		if action != "unknown" || choice != nil || !strings.Contains(reply, says) {
			t.Errorf("%s: action = %s card = %+v reply = %q", want, action, choice, reply)
		}
	}
	// "cheapest" names the cheapest usable service on the button.
	if action, _, choice := askSwitch(t, s, &answer, "cheapest"); action != "switch_video_service" || choice == nil || choice.Mode != "cheapest" || choice.Provider != "openrouter" {
		t.Fatalf("cheapest: %s %+v", action, choice)
	}
	// The route refuses a service that cannot make videos, in plain words for the card.
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"`+switchFamily+`","provider":"fal"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "no balance left") {
		t.Fatalf("manual fal: %d %s", w.Code, w.Body.String())
	}

	// Nothing else usable: no button, and the reply says what to fix.
	s.database.SetProviderSecret("openrouter", "", nil)
	action, reply, choice := askSwitch(t, s, &answer, "cheapest")
	if action != "unknown" || choice != nil || !strings.Contains(reply, "ArgoLink did not accept this key") || !strings.Contains(reply, "No other connected video service") {
		t.Fatalf("nothing usable: %s %+v %q", action, choice, reply)
	}
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"cheapest","model":"`+switchFamily+`"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "add a working key") {
		t.Fatalf("cheapest with nothing usable: %d %s", w.Code, w.Body.String())
	}
}

func TestPlannerIsToldToOfferTheSwitch(t *testing.T) {
	answer := `{"reply":"ok","action":"none"}`
	s := routerServer(t, nil)
	system, _ := plannerSays(s, &answer)
	sendChat(t, s, "hi")
	for _, want := range []string{"switch_video_service", "can_switch_to", `"cheapest"`, "**Use <service> for videos**"} {
		if !strings.Contains(*system, want) {
			t.Errorf("prompt is missing %s", want)
		}
	}
}
