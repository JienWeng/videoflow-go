package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"videoflow-go/internal/models"
)

// The canvas shows how a story becomes a video. On top of the stored links, storyGraph
// says which episode each scene belongs to, names each scene video after its scene, and
// adds one "final video" box per episode export with a link from every scene video it
// joins. Only the newest export of an episode is shown.
func (s *Server) storyGraph(project string, g *models.GraphResponse) {
	episodes := map[string]seriesEpisode{}
	for _, e := range s.seriesEpisodes(project) {
		episodes[e.ScriptID] = e
	}
	scenes := map[string]models.Scene{}
	all, _ := s.database.ListScenes(project)
	for _, scene := range all {
		scenes[scene.ID] = scene
	}
	episodeOf := func(sceneID string) (seriesEpisode, bool) {
		scene, ok := scenes[sceneID]
		if !ok || scene.ScriptID == nil {
			return seriesEpisode{}, false
		}
		e, ok := episodes[*scene.ScriptID]
		return e, ok
	}
	text := func(v any) string {
		switch t := v.(type) {
		case string:
			return t
		case *string:
			if t != nil {
				return *t
			}
		}
		return ""
	}
	jobScene, outputScene := map[string]string{}, map[string]string{}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		switch n.Type {
		case "scene":
			if e, ok := episodeOf(n.ID); ok {
				n.Data["script_id"], n.Data["episode_number"], n.Data["episode_title"] = e.ScriptID, e.Number, e.Title
			}
		case "render_job":
			jobScene[n.ID] = text(n.Data["scene_id"])
		}
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.Type != "output" {
			continue
		}
		sceneID := jobScene[text(n.Data["render_job_id"])]
		if scene, ok := scenes[sceneID]; ok {
			outputScene[n.ID] = sceneID
			n.Label = scene.Title + " · video"
			n.Data["scene_id"], n.Data["scene_title"] = sceneID, scene.Title
			if e, ok := episodeOf(sceneID); ok {
				n.Data["script_id"], n.Data["episode_number"], n.Data["episode_title"] = e.ScriptID, e.Number, e.Title
			}
		}
	}

	assets, _ := s.database.ListAssets(project)
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].CreatedAt.After(assets[j].CreatedAt) })
	shown := map[string]bool{}
	for _, a := range assets {
		var meta struct {
			Revision string `json:"composition_revision"`
			Sources  []struct {
				AssetID string `json:"asset_id"`
			} `json:"source_assets"`
		}
		if a.Type != "output_video" || json.Unmarshal(a.MetadataJSON, &meta) != nil || meta.Revision == "" {
			continue
		}
		joined, titles, scripts := []string{}, []string{}, map[string]bool{}
		for _, source := range meta.Sources {
			id := strings.TrimPrefix(source.AssetID, "output-")
			sceneID, ok := outputScene[id]
			if !ok {
				continue
			}
			joined, titles = append(joined, id), append(titles, scenes[sceneID].Title)
			if e, ok := episodeOf(sceneID); ok {
				scripts[e.ScriptID] = true
			}
		}
		if len(joined) == 0 {
			continue // made from clips that are no longer on the canvas
		}
		data := map[string]any{"file_path": a.FilePath, "name": a.Name, "created_at": a.CreatedAt, "scene_titles": titles, "scene_count": len(titles), "revision": meta.Revision}
		label, key := "Final video", "mixed:"+strings.Join(joined, ",")
		if len(scripts) == 1 {
			for script := range scripts {
				e := episodes[script]
				label, key = fmt.Sprintf("Episode %d · %s", e.Number, e.Title), script
				data["script_id"], data["episode_number"], data["episode_title"] = script, e.Number, e.Title
			}
		}
		if shown[key] {
			continue // an older export of the same episode
		}
		shown[key] = true
		g.Nodes = append(g.Nodes, models.GraphNode{ID: a.ID, Type: "final_video", Label: label, Data: data})
		for _, id := range joined {
			g.Edges = append(g.Edges, models.GraphEdge{Source: id, Target: a.ID, Label: "joined"})
		}
	}
}
