package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Costs: what each paid service has charged for this project, and what the next
// step will cost before it runs.

// falPlatformURL is the fal.ai account API (prices and billed usage). Tests replace it.
var falPlatformURL = "https://api.fal.ai/v1"

func (s *Server) registerCostRoutes() {
	s.router.Get("/costs", s.handleCosts)
	s.router.Get("/costs/estimate", s.handleCostEstimate)
	providers.OnTextCost = func(model string, tokens, dollars float64) {
		project, err := s.database.GetActiveProjectID()
		if err != nil {
			return
		}
		_ = s.database.RecordCost(db.CostEntry{Ref: "text:" + uuid.New().String()[:12], ProjectID: project, Category: "text", Provider: "openrouter", Model: model, Label: "Picture and reference analysis", Quantity: tokens, Unit: "tokens", Amount: dollars, Basis: costs.Metered})
	}
	if projects, err := s.database.ListProjects(); err == nil {
		for _, p := range projects {
			_ = s.database.SyncCosts(p.ID)
		}
	}
}

func (s *Server) falPlatform(ctx context.Context, path string, target any) error {
	key, _, _ := s.effectiveProviderKey("fal")
	if key == "" {
		return fmt.Errorf("fal is not connected")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", falPlatformURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Key "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("fal HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(body, target)
}

// refreshVideoPrices asks fal for today's unit price of each video model, at most daily.
func (s *Server) refreshVideoPrices(ctx context.Context, wanted ...string) {
	ids := []string{}
	for _, model := range wanted {
		if model == "" {
			continue
		}
		if at, err := s.database.GetAppSetting("price_checked_" + model); err == nil {
			if t, e := time.Parse(time.RFC3339, at); e == nil && time.Since(t) < 24*time.Hour {
				continue
			}
		}
		ids = append(ids, model)
	}
	if len(ids) == 0 {
		return
	}
	var result struct {
		Prices []struct {
			Endpoint  string  `json:"endpoint_id"`
			UnitPrice float64 `json:"unit_price"`
			Unit      string  `json:"unit"`
		} `json:"prices"`
	}
	if s.falPlatform(ctx, "/models/pricing?endpoint_id="+url.QueryEscape(strings.Join(ids, ",")), &result) != nil {
		return
	}
	for _, p := range result.Prices {
		raw, _ := json.Marshal(costs.LivePrice{UnitPrice: p.UnitPrice, Unit: p.Unit})
		_ = s.database.SetAppSetting("price_"+p.Endpoint, string(raw))
	}
	for _, id := range ids {
		_ = s.database.SetAppSetting("price_checked_"+id, time.Now().UTC().Format(time.RFC3339))
	}
}

func (s *Server) videoPrice(model string) *costs.LivePrice {
	raw, err := s.database.GetAppSetting("price_" + model)
	if err != nil {
		return nil
	}
	var p costs.LivePrice
	if json.Unmarshal([]byte(raw), &p) != nil || p.UnitPrice <= 0 {
		return nil
	}
	return &p
}

// falBilled returns what fal's own billing says the whole account was charged since a date.
// It needs a key allowed to read billing; without one it reports ok=false.
func (s *Server) falBilled(ctx context.Context, since time.Time) (total float64, ok bool) {
	cursor := ""
	for page := 0; page < 10; page++ {
		var result struct {
			Series []struct {
				Results []struct {
					Total float64 `json:"cost_total"`
				} `json:"results"`
			} `json:"time_series"`
			Next string `json:"next_cursor"`
			More bool   `json:"has_more"`
		}
		q := url.Values{"start": {since.UTC().Format("2006-01-02")}, "timeframe": {"day"}, "limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if s.falPlatform(ctx, "/models/usage?"+q.Encode(), &result) != nil {
			return 0, false
		}
		for _, bucket := range result.Series {
			for _, line := range bucket.Results {
				total += line.Total
			}
		}
		if !result.More || result.Next == "" {
			return money(total), true
		}
		cursor = result.Next
	}
	return money(total), true
}

func money(v float64) float64 { return math.Round(v*100) / 100 }

type costGroup struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Amount   float64 `json:"amount"`
	Count    int     `json:"count"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Detail   string  `json:"detail"`
	Exact    bool    `json:"exact"`
}

var costLabels = map[string]string{"video": "Video clips", "image": "Pictures", "voice": "Voice", "text": "Text analysis"}
var serviceLabels = map[string]string{"fal": "fal.ai", "openai": "OpenAI", "fish": "Fish Audio", "elevenlabs": "ElevenLabs", "openrouter": "OpenRouter", "codex": "Codex", "claude": "Claude Code", "byteplus": "BytePlus"}

func serviceLabel(provider string) string {
	if l := serviceLabels[provider]; l != "" {
		return l
	}
	return provider
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	if _, model, e := s.productionVideoModel(r.Context()); e == nil {
		s.refreshVideoPrices(r.Context(), model.ID)
	}
	_ = s.database.SyncCosts(project)
	entries, err := s.database.ListCosts(project)
	if err != nil {
		http.Error(w, "couldn't read the costs", 500)
		return
	}
	episodes := s.seriesEpisodes(project)
	episodeOf := map[string]*costGroup{}
	episodeList := []*costGroup{}
	for _, e := range episodes {
		g := &costGroup{Key: e.ScriptID, Label: fmt.Sprintf("Episode %d: %s", e.Number, e.Title), Exact: true}
		if len(episodes) == 1 {
			g.Label = e.Title
		}
		episodeOf[e.ScriptID] = g
		episodeList = append(episodeList, g)
	}
	other := &costGroup{Key: "", Label: "Shared across episodes (cast, places, props, voice)", Exact: true}
	categories, services := map[string]*costGroup{}, map[string]*costGroup{}
	included := map[string]*costGroup{}
	unconfirmed, unpriced := []db.CostEntry{}, 0
	total, uncertain := 0.0, 0.0
	var first time.Time
	hasFal := false
	for _, e := range entries {
		if first.IsZero() || e.CreatedAt.Before(first) {
			first = e.CreatedAt
		}
		switch e.Basis {
		case "unconfirmed":
			unconfirmed = append(unconfirmed, e)
			continue
		case costs.Included:
			key := e.Category + ":" + e.Provider
			g := included[key]
			if g == nil {
				g = &costGroup{Key: key, Label: costLabels[e.Category] + " made by " + serviceLabel(e.Provider), Unit: e.Unit}
				included[key] = g
			}
			g.Count++
			g.Quantity += e.Quantity
			continue
		case costs.Unpriced:
			unpriced++
			continue
		}
		hasFal = hasFal || e.Provider == "fal"
		total += e.Amount
		if e.Basis == costs.Estimate {
			uncertain += e.Amount
		}
		add := func(g *costGroup) {
			g.Amount += e.Amount
			g.Count++
			g.Quantity += e.Quantity
			g.Exact = g.Exact && e.Basis != costs.Estimate
		}
		c := categories[e.Category]
		if c == nil {
			c = &costGroup{Key: e.Category, Label: costLabels[e.Category], Unit: e.Unit, Exact: true}
			categories[e.Category] = c
		}
		add(c)
		sv := services[e.Provider]
		if sv == nil {
			sv = &costGroup{Key: e.Provider, Label: serviceLabel(e.Provider), Exact: true}
			services[e.Provider] = sv
		}
		add(sv)
		sv.Quantity = 0
		if g := episodeOf[e.ScriptID]; g != nil {
			add(g)
		} else {
			add(other)
		}
	}
	flat := func(m map[string]*costGroup, order []string) []costGroup {
		out := []costGroup{}
		seen := map[string]bool{}
		for _, k := range order {
			if g := m[k]; g != nil {
				seen[k] = true
				g.Amount = money(g.Amount)
				out = append(out, *g)
			}
		}
		rest := []string{}
		for k := range m {
			if !seen[k] {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		for _, k := range rest {
			m[k].Amount = money(m[k].Amount)
			out = append(out, *m[k])
		}
		return out
	}
	if other.Count > 0 {
		episodeList = append(episodeList, other)
	}
	byEpisode := []costGroup{}
	for _, g := range episodeList {
		g.Amount = money(g.Amount)
		byEpisode = append(byEpisode, *g)
	}
	free := flat(included, nil)
	if replies, e := s.database.AssistantReplies(project); e == nil {
		for _, engine := range []string{"claude", "codex"} {
			if n := replies[engine]; n > 0 {
				free = append(free, costGroup{Key: "assistant:" + engine, Label: "Assistant replies from " + serviceLabel(engine), Count: n, Unit: "replies"})
			}
		}
	}
	out := map[string]any{
		"currency": "USD", "total": money(total), "estimated": money(uncertain),
		"categories": flat(categories, []string{"video", "image", "voice", "text"}),
		"services":   flat(services, []string{"fal", "openai", "fish"}),
		"episodes":   byEpisode, "included": free,
		"unconfirmed": unconfirmed, "unpriced": unpriced, "items": entries,
	}
	if !first.IsZero() {
		out["since"] = first
	}
	// Every project side by side, so the whole spend is visible from any of them.
	projects, _ := s.database.ListProjects()
	for _, p := range projects {
		if p.ID != project {
			_ = s.database.SyncCosts(p.ID)
		}
	}
	if totals, e := s.database.CostTotals(); e == nil {
		all, sum := []costGroup{}, 0.0
		for _, p := range projects {
			if amount := totals[p.ID]; amount > 0 {
				all = append(all, costGroup{Key: p.ID, Label: p.Name, Amount: money(amount), Exact: p.ID == project})
				sum += amount
				delete(totals, p.ID)
			}
		}
		gone := 0.0
		for _, amount := range totals {
			gone += amount
		}
		if gone > 0 {
			all = append(all, costGroup{Key: "deleted", Label: "Deleted projects", Amount: money(gone)})
			sum += gone
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].Amount > all[j].Amount })
		out["projects"], out["all_total"] = all, money(sum)
	}
	if hasFal && r.URL.Query().Get("reconcile") == "1" {
		if billed, ok := s.falBilled(r.Context(), first); ok {
			out["fal_billed"] = map[string]any{"amount": billed, "since": first}
		} else {
			out["fal_billed"] = map[string]any{"unavailable": true}
		}
	}
	writeJSON(w, 200, out)
}

// handleCostEstimate prices finishing one story before any money is spent.
func (s *Server) handleCostEstimate(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	scriptID := r.URL.Query().Get("script_id")
	scenes := []models.Scene{}
	all, _ := s.database.ListScenes(project)
	for _, scene := range all {
		if scene.ScriptID != nil && *scene.ScriptID == scriptID {
			scenes = append(scenes, scene)
		}
	}
	if len(scenes) == 0 {
		http.Error(w, "that story has no scenes in this project", 404)
		return
	}
	sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].SceneOrder < scenes[j].SceneOrder })
	type line struct {
		SceneID string  `json:"scene_id"`
		Title   string  `json:"title"`
		Seconds int     `json:"seconds"`
		Clips   int     `json:"clips"`
		Amount  float64 `json:"amount"`
		Note    string  `json:"note,omitempty"`
	}
	lines, total, seconds, known := []line{}, 0.0, 0, true
	model := ""
	for i := range scenes {
		scene := &scenes[i]
		plan, e := s.sceneProductionPlan(r.Context(), scene)
		l := line{SceneID: scene.ID, Title: scene.Title}
		if e != nil {
			l.Note = "Not ready for video yet"
			lines = append(lines, l)
			continue
		}
		if model == "" {
			model = plan.Model
			s.refreshVideoPrices(r.Context(), model)
		}
		l.Seconds, l.Clips = plan.NewSeconds, plan.Calls
		if plan.Blocked != "" {
			l.Note = plan.Blocked
		} else if plan.Calls == 0 {
			l.Note = "Already made"
		} else {
			amount, basis := costs.Video(plan.Provider, plan.Model, plan.Resolution, scene.AspectRatio, float64(plan.NewSeconds), s.videoPrice(plan.Model))
			if basis == costs.Unpriced {
				known, l.Note = false, "No known price for this video model"
			}
			l.Amount = money(amount)
			total += amount
			seconds += plan.NewSeconds
		}
		lines = append(lines, l)
	}
	writeJSON(w, 200, map[string]any{"currency": "USD", "total": money(total), "seconds": seconds, "model": model, "known": known, "scenes": lines})
}
