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
	if l := costs.Retired[provider]; l != "" {
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
	s.refreshVideoPrices(r.Context(), false)
	_ = s.database.SyncCosts(project)
	entries, err := s.database.ListCosts(project)
	if err != nil {
		http.Error(w, "couldn't read the costs", 500)
		return
	}
	sum := s.newCostSummary(project)
	for _, e := range entries {
		sum.add(e)
	}
	out := map[string]any{
		"currency": "USD", "total": money(sum.total), "estimated": money(sum.uncertain),
		"categories": flatCostGroups(sum.categories, []string{"video", "image", "voice", "text"}),
		"services":   flatCostGroups(sum.services, []string{"fal", "openai", "fish"}),
		"episodes":   sum.byEpisode(), "included": s.includedCosts(project, sum.included),
		"unconfirmed": sum.unconfirmed, "unpriced": sum.unpriced, "items": entries,
	}
	if !sum.first.IsZero() {
		out["since"] = sum.first
	}
	s.addAllProjectCosts(out, project)
	if sum.hasFal && r.URL.Query().Get("reconcile") == "1" {
		if billed, ok := s.falBilled(r.Context(), sum.first); ok {
			out["fal_billed"] = map[string]any{"amount": billed, "since": sum.first}
		} else {
			out["fal_billed"] = map[string]any{"unavailable": true}
		}
	}
	writeJSON(w, 200, out)
}

// costSummary totals one project's cost entries by category, service and episode.
type costSummary struct {
	episodeOf                      map[string]*costGroup
	episodes                       []*costGroup
	other                          *costGroup
	categories, services, included map[string]*costGroup
	unconfirmed                    []db.CostEntry
	unpriced                       int
	total, uncertain               float64
	first                          time.Time
	hasFal                         bool
}

func (s *Server) newCostSummary(project string) *costSummary {
	sum := &costSummary{
		episodeOf:   map[string]*costGroup{},
		episodes:    []*costGroup{},
		other:       &costGroup{Key: "", Label: "Shared across episodes (cast, places, props, voice)", Exact: true},
		categories:  map[string]*costGroup{},
		services:    map[string]*costGroup{},
		included:    map[string]*costGroup{},
		unconfirmed: []db.CostEntry{},
	}
	episodes := s.seriesEpisodes(project)
	for _, e := range episodes {
		g := &costGroup{Key: e.ScriptID, Label: fmt.Sprintf("Episode %d: %s", e.Number, e.Title), Exact: true}
		if len(episodes) == 1 {
			g.Label = e.Title
		}
		sum.episodeOf[e.ScriptID] = g
		sum.episodes = append(sum.episodes, g)
	}
	return sum
}

// add counts one entry: set aside if unconfirmed, unpriced or included in a plan,
// otherwise added to its category, service and episode.
func (sum *costSummary) add(e db.CostEntry) {
	if sum.first.IsZero() || e.CreatedAt.Before(sum.first) {
		sum.first = e.CreatedAt
	}
	switch e.Basis {
	case "unconfirmed":
		sum.unconfirmed = append(sum.unconfirmed, e)
		return
	case costs.Included:
		key := e.Category + ":" + e.Provider
		g := sum.included[key]
		if g == nil {
			g = &costGroup{Key: key, Label: costLabels[e.Category] + " made by " + serviceLabel(e.Provider), Unit: e.Unit}
			sum.included[key] = g
		}
		g.Count++
		g.Quantity += e.Quantity
		return
	case costs.Unpriced:
		sum.unpriced++
		return
	}
	sum.hasFal = sum.hasFal || e.Provider == "fal"
	sum.total += e.Amount
	if e.Basis == costs.Estimate {
		sum.uncertain += e.Amount
	}
	add := func(g *costGroup) {
		g.Amount += e.Amount
		g.Count++
		g.Quantity += e.Quantity
		g.Exact = g.Exact && e.Basis != costs.Estimate
	}
	c := sum.categories[e.Category]
	if c == nil {
		c = &costGroup{Key: e.Category, Label: costLabels[e.Category], Unit: e.Unit, Exact: true}
		sum.categories[e.Category] = c
	}
	add(c)
	sv := sum.services[e.Provider]
	if sv == nil {
		sv = &costGroup{Key: e.Provider, Label: serviceLabel(e.Provider), Exact: true}
		sum.services[e.Provider] = sv
	}
	add(sv)
	sv.Quantity = 0
	if g := sum.episodeOf[e.ScriptID]; g != nil {
		add(g)
	} else {
		add(sum.other)
	}
}

// byEpisode lists each episode's spend, then what is shared across them.
func (sum *costSummary) byEpisode() []costGroup {
	list := sum.episodes
	if sum.other.Count > 0 {
		list = append(list, sum.other)
	}
	out := []costGroup{}
	for _, g := range list {
		g.Amount = money(g.Amount)
		out = append(out, *g)
	}
	return out
}

// flatCostGroups lists groups in the given order, then the rest by key, with amounts
// rounded to cents.
func flatCostGroups(m map[string]*costGroup, order []string) []costGroup {
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

// includedCosts lists work covered by a plan, including assistant replies.
func (s *Server) includedCosts(project string, included map[string]*costGroup) []costGroup {
	free := flatCostGroups(included, nil)
	if replies, e := s.database.AssistantReplies(project); e == nil {
		for _, engine := range []string{"claude", "codex"} {
			if n := replies[engine]; n > 0 {
				free = append(free, costGroup{Key: "assistant:" + engine, Label: "Assistant replies from " + serviceLabel(engine), Count: n, Unit: "replies"})
			}
		}
	}
	return free
}

// addAllProjectCosts puts every project side by side, so the whole spend is visible
// from any of them.
func (s *Server) addAllProjectCosts(out map[string]any, project string) {
	projects, _ := s.database.ListProjects()
	for _, p := range projects {
		if p.ID != project {
			_ = s.database.SyncCosts(p.ID)
		}
	}
	// The report view filters and totals these itself, across one project or all of them.
	projectNames, episodeNames := map[string]string{}, map[string]string{}
	for _, p := range projects {
		projectNames[p.ID] = p.Name
		list := s.seriesEpisodes(p.ID)
		for _, e := range list {
			episodeNames[e.ScriptID] = e.Title
			if len(list) > 1 {
				episodeNames[e.ScriptID] = fmt.Sprintf("Episode %d: %s", e.Number, e.Title)
			}
		}
	}
	out["project_id"], out["project_names"], out["episode_names"], out["generated_at"] = project, projectNames, episodeNames, time.Now().UTC()
	if everything, e := s.database.ListCosts(""); e == nil {
		out["all_items"] = everything
	}
	if totals, e := s.database.CostTotals(); e == nil {
		out["projects"], out["all_total"] = projectCostTotals(projects, project, totals)
	}
}

// projectCostTotals ranks projects by spend, with deleted projects together at the end
// of the ranking, and returns the grand total.
func projectCostTotals(projects []models.Project, project string, totals map[string]float64) ([]costGroup, float64) {
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
	return all, money(sum)
}

// handleCostEstimate prices finishing one story before any money is spent.
func (s *Server) handleCostEstimate(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	// The same scenes an autopilot run with these inputs would make.
	query := autopilotRequest{ScriptID: r.URL.Query().Get("script_id"), VideosOnly: true}
	if ids := strings.TrimSpace(r.URL.Query().Get("scene_ids")); ids != "" {
		query.SceneIDs = strings.Split(ids, ",")
	}
	scenes, code, problem := s.autopilotScenes(project, query)
	if code != 0 {
		http.Error(w, problem, code)
		return
	}
	type line struct {
		SceneID string  `json:"scene_id"`
		Title   string  `json:"title"`
		Seconds int     `json:"seconds"`
		Clips   int     `json:"clips"`
		Amount  float64 `json:"amount"`
		Note    string  `json:"note,omitempty"`
	}
	s.refreshVideoPrices(r.Context(), false)
	settings, _ := s.database.AppSettingsSnapshot()
	live := costs.LiveFromSettings(settings)
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
		}
		l.Seconds, l.Clips = plan.NewSeconds, plan.Calls
		if plan.Blocked != "" {
			l.Note = plan.Blocked
		} else if plan.Calls == 0 {
			l.Note = "Already made"
		} else {
			amount, basis := costs.Video(plan.Provider, plan.Model, plan.Resolution, scene.AspectRatio, float64(plan.NewSeconds), live)
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
