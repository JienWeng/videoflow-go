package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/routing"
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
}

func (s *Server) videoServices(r *http.Request, family, resolution string, seconds float64) map[string]any {
	routes := s.routes()
	if fal := costs.HostByName("fal"); fal != nil && routes.VideoConnected("fal") {
		s.refreshVideoPrices(r.Context(), fal.Models["seedance-2.0"], fal.Models["seedance-2.0-fast"])
	}
	s.refreshOpenRouterVideoPrices(r.Context())
	settings, _ := s.database.AppSettingsSnapshot()
	text := func(key, fallback string) string {
		var value string
		if json.Unmarshal([]byte(settings[key]), &value) == nil && value != "" {
			return value
		}
		return fallback
	}
	if family == "" {
		family = text("video_family", costs.Families[0].ID)
	}
	if resolution == "" {
		resolution = text("default_resolution", "720p")
	}
	provider, model := routes.MediaSelection("video")
	list, picked := []videoService{}, false
	for _, offer := range costs.Offers(family, resolution, "16:9", routing.LiveVideoPrices(settings)) {
		v := videoService{Offer: offer, Total: money(offer.PerSecond * seconds), Connected: routes.VideoConnected(offer.Provider), InUse: offer.Provider == provider && offer.Model == model}
		switch {
		case !offer.Frames:
			v.Note = "Text prompts only. It cannot start a clip from your storyboard picture, so scenes cannot use it."
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
	current := map[string]any{"provider": provider, "model": model, "label": serviceLabel(provider), "family": costs.FamilyOf(provider, model)}
	if host := costs.HostByName(provider); host != nil {
		current["label"] = host.Label
	}
	return map[string]any{"families": costs.Families, "model": family, "resolution": resolution, "seconds": seconds, "routing": text("video_routing", "manual"), "current": current, "services": list}
}

// openRouterVideoFeed is OpenRouter's public list of video models and prices. Tests replace it.
var openRouterVideoFeed = "https://openrouter.ai/api/v1/videos/models"

// refreshOpenRouterVideoPrices reads OpenRouter's public price list at most once a day.
func (s *Server) refreshOpenRouterVideoPrices(ctx context.Context) {
	if at, err := s.database.GetAppSetting("video_rates_checked_openrouter"); err == nil {
		if t, e := time.Parse(time.RFC3339, at); e == nil && time.Since(t) < 24*time.Hour {
			return
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", openRouterVideoFeed, nil)
	if err != nil {
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	var feed struct {
		Data []struct {
			ID   string `json:"id"`
			SKUs struct {
				Tokens string `json:"video_tokens"`
			} `json:"pricing_skus"`
		} `json:"data"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&feed) != nil {
		return
	}
	host := costs.HostByName("openrouter")
	for _, m := range feed.Data {
		if family := costs.FamilyOf("openrouter", m.ID); family != "" && host != nil {
			if perToken, e := strconv.ParseFloat(m.SKUs.Tokens, 64); e == nil && perToken > 0 {
				_ = s.database.SetAppSetting("video_rate_openrouter:"+family, strconv.FormatFloat(perToken*1000, 'f', -1, 64))
			}
		}
	}
	_ = s.database.SetAppSetting("video_rates_checked_openrouter", time.Now().UTC().Format(time.RFC3339))
}

// chatVideoService tells the assistant which service makes the videos and what it charges.
func (s *Server) chatVideoService() map[string]any {
	provider, model := s.routes().MediaSelection("video")
	out := map[string]any{"service": serviceLabel(provider), "picked": "by the user"}
	if host := costs.HostByName(provider); host != nil {
		out["service"] = host.Label
	}
	if s.routes().Setting("video_routing", "manual") == "cheapest" {
		out["picked"] = "automatically, the cheapest connected service"
	}
	settings, _ := s.database.AppSettingsSnapshot()
	family := costs.FamilyOf(provider, model)
	if price, ok := costs.HostPrice(provider, family, "720p", "16:9", routing.LiveVideoPrices(settings)[provider+":"+family]); ok {
		out["dollars_per_second_720p"] = money(price)
	}
	return out
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
	if body.Mode == "manual" && body.Provider != "" {
		host := costs.HostByName(body.Provider)
		if host == nil || host.Models[body.Model] == "" || !host.Frames {
			http.Error(w, "that service does not offer this model for storyboard scenes", 400)
			return
		}
		if !s.routes().VideoConnected(body.Provider) {
			http.Error(w, "add an API key for "+host.Label+" first", 400)
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
