package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
)

// Video services: the same video model is sold by many services at different prices.
// These routes compare them and let the app always use the cheapest connected one.

func (s *Server) registerVideoServiceRoutes() {
	s.router.Get("/video-services", s.handleVideoServices)
	s.router.Put("/video-services/routing", s.handleVideoRouting)
}

type videoService struct {
	costs.Offer
	Total     float64 `json:"total"`
	Connected bool    `json:"connected"`
	Usable    bool    `json:"usable"`
	Cheapest  bool    `json:"cheapest"`
	InUse     bool    `json:"in_use"`
	Note      string  `json:"note,omitempty"`
	// CheckedAt is when the price was last read from the service itself; empty means never.
	CheckedAt string `json:"checked_at,omitempty"`
	Problem   string `json:"problem,omitempty"`
	// KeyCheck is what the service last said about the saved key.
	KeyCheck *providers.KeyCheck `json:"key_check,omitempty"`
}

func (s *Server) videoServices(r *http.Request, family, resolution string, seconds float64) map[string]any {
	s.refreshVideoPrices(r.Context(), r.URL.Query().Get("refresh") == "1")
	s.refreshKeyChecks()
	settings, _ := s.database.AppSettingsSnapshot()
	text := func(key, fallback string) string { return settingText(settings, key, fallback) }
	if family == "" {
		family = text("video_family", costs.Families[0].ID)
	}
	if resolution == "" {
		resolution = text("default_resolution", "720p")
	}
	provider, model := s.routes().MediaSelection("video")
	current := map[string]any{"provider": provider, "model": model, "label": videoServiceLabel(provider), "family": costs.FamilyOf(provider, model)}
	return map[string]any{"families": costs.Families, "model": family, "resolution": resolution, "seconds": seconds, "routing": text("video_routing", "manual"), "current": current, "services": s.videoOffers(family, resolution, seconds, settings)}
}

func settingText(settings map[string]string, key, fallback string) string {
	var value string
	if json.Unmarshal([]byte(settings[key]), &value) == nil && value != "" {
		return value
	}
	return fallback
}

// videoOffers lists every service that sells the family at the resolution, cheapest
// first, and whether each can make scene videos now. It reads saved state only.
func (s *Server) videoOffers(family, resolution string, seconds float64, settings map[string]string) []videoService {
	routes := s.routes()
	provider, model := routes.MediaSelection("video")
	list, picked := []videoService{}, false
	for _, offer := range costs.Offers(family, resolution, "16:9", costs.LiveFromSettings(settings)) {
		v := videoService{Offer: offer, Total: money(offer.PerSecond * seconds), Connected: routes.Connected(offer.Provider), KeyCheck: s.keyCheckView(offer.Provider), InUse: offer.Provider == provider && offer.Model == model}
		v.CheckedAt, v.Problem = settings["video_checked_"+offer.Provider], settings["video_problem_"+offer.Provider]
		switch {
		case !offer.Frames:
			v.Note = "Text prompts only. It cannot start a clip from your storyboard picture, so scenes cannot use it."
		case s.serviceProblem(offer.Provider) != "":
			v.Note = s.serviceProblem(offer.Provider)
		case !v.Connected:
			v.Note = "Add its API key to use it."
		default:
			v.Usable = true
			if !picked {
				v.Cheapest, picked = true, true
			}
		}
		list = append(list, v)
	}
	return list
}

// chatVideoFamily is the video model the assistant prices and switches: the one in use,
// or the saved choice when the service in use is not a known host.
func (s *Server) chatVideoFamily(settings map[string]string) string {
	if family := costs.FamilyOf(s.routes().MediaSelection("video")); family != "" {
		return family
	}
	return settingText(settings, "video_family", costs.Families[0].ID)
}

// chatVideoService tells the assistant which service makes the videos, what it charges,
// and which other services could make them now (cheapest first).
func (s *Server) chatVideoService() map[string]any {
	provider, model := s.routes().MediaSelection("video")
	out := map[string]any{"service": videoServiceLabel(provider), "picked": "by the user"}
	if s.routes().Setting("video_routing", "manual") == "cheapest" {
		out["picked"] = "automatically, the cheapest connected service"
	}
	if problem := s.serviceProblem(provider); problem != "" {
		out["problem"] = problem
	}
	settings, _ := s.database.AppSettingsSnapshot()
	family := costs.FamilyOf(provider, model)
	if price, ok := costs.HostPrice(provider, family, "720p", "16:9", costs.LiveFromSettings(settings)); ok {
		out["dollars_per_second_720p"] = money(price)
	}
	others := []map[string]any{}
	for _, v := range s.videoOffers(s.chatVideoFamily(settings), "720p", 1, settings) {
		if v.Usable && !v.InUse && len(others) < 3 {
			others = append(others, map[string]any{"id": v.Provider, "service": v.Label, "dollars_per_second_720p": v.PerSecond})
		}
	}
	if len(others) > 0 {
		out["can_switch_to"] = others
	}
	return out
}

// chatVideoSwitch checks the service the planner offers to switch to (a service id or
// label, or "cheapest"). It returns the button's routing request, or the plain reason
// no switch can be offered.
func (s *Server) chatVideoSwitch(want string) (map[string]any, string) {
	settings, _ := s.database.AppSettingsSnapshot()
	family := s.chatVideoFamily(settings)
	want = strings.TrimSpace(want)
	cheapest := strings.EqualFold(want, "cheapest")
	for _, v := range s.videoOffers(family, "720p", 1, settings) {
		if cheapest && !v.Usable || !cheapest && !strings.EqualFold(want, v.Provider) && !strings.EqualFold(want, v.Label) {
			continue
		}
		if !v.Usable {
			return nil, v.Label + " can't make your videos right now. " + v.Note
		}
		if v.InUse && !cheapest {
			return nil, v.Label + " already makes your videos, so there is nothing to switch."
		}
		mode := "manual"
		if cheapest {
			mode = "cheapest"
		}
		return map[string]any{"mode": mode, "model": family, "provider": v.Provider, "label": v.Label, "per_second": v.PerSecond}, ""
	}
	fix := "No other connected video service can make these videos yet."
	if provider, _ := s.routes().MediaSelection("video"); s.serviceProblem(provider) != "" {
		fix = s.serviceProblem(provider) + " " + fix
	}
	return nil, fix + " Add a working key for a video service, then ask me again."
}

func (s *Server) handleVideoServices(w http.ResponseWriter, r *http.Request) {
	seconds, _ := strconv.ParseFloat(r.URL.Query().Get("seconds"), 64)
	if seconds < 1 || seconds > 600 {
		seconds = 10
	}
	writeJSON(w, 200, s.videoServices(r, r.URL.Query().Get("model"), r.URL.Query().Get("resolution"), seconds))
}

// handleVideoRouting chooses how the video service is picked: "cheapest" re-checks prices
// for the chosen model every time a video is made; "manual" pins one service.
func (s *Server) handleVideoRouting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode     string `json:"mode"`
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body) != nil || (body.Mode != "cheapest" && body.Mode != "manual") {
		http.Error(w, "choose cheapest or manual", 400)
		return
	}
	known := false
	for _, f := range costs.Families {
		known = known || f.ID == body.Model
	}
	if !known {
		http.Error(w, "choose a video model", 400)
		return
	}
	save := func(key, value string) error {
		raw, _ := json.Marshal(value)
		return s.database.SetAppSetting(key, string(raw))
	}
	if body.Mode == "manual" && body.Provider == "" {
		// No service named: keep the one in use, moved to the chosen model.
		body.Provider, _ = s.routes().MediaSelection("video")
	}
	if body.Mode == "cheapest" {
		settings, _ := s.database.AppSettingsSnapshot()
		usable := false
		for _, v := range s.videoOffers(body.Model, settingText(settings, "default_resolution", "720p"), 1, settings) {
			usable = usable || v.Usable
		}
		if !usable {
			http.Error(w, "none of your connected video services can make videos from your pictures; add a working key for one first", 400)
			return
		}
	}
	if body.Mode == "manual" {
		host := costs.HostByName(body.Provider)
		if host == nil || host.Models[body.Model] == "" || !host.Frames {
			http.Error(w, "the current video service does not offer this model; pick a service that does, or use the cheapest connected one", 400)
			return
		}
		if !s.routes().Connected(body.Provider) {
			problem := s.serviceProblem(body.Provider)
			if problem == "" {
				problem = "add an API key for " + host.Label + " first"
			}
			http.Error(w, problem, 400)
			return
		}
		if save("default_video_provider", body.Provider) != nil || save("video_model", host.Models[body.Model]) != nil {
			http.Error(w, "couldn't save the choice", 500)
			return
		}
	}
	if save("video_routing", body.Mode) != nil || save("video_family", body.Model) != nil {
		http.Error(w, "couldn't save the choice", 500)
		return
	}
	writeJSON(w, 200, s.videoServices(r, body.Model, "", 10))
}
