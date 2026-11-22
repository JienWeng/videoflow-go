package db

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"videoflow-go/internal/models"
)

// --- Graph Visualization ---

// graphBuilder collects the project's graph nodes and edges in the order they are found.
type graphBuilder struct {
	nodes []models.GraphNode
	edges []models.GraphEdge
}

func (d *DB) GetGraphData(projectID string) (*models.GraphResponse, error) {
	g := &graphBuilder{nodes: []models.GraphNode{}, edges: []models.GraphEdge{}}

	activeReferenceIDs := d.addCharacterNodes(g, projectID)
	d.addAssetNodes(g, projectID, activeReferenceIDs)
	d.addSceneNodes(g, projectID)
	d.addRenderNodes(g, projectID)

	return &models.GraphResponse{
		Nodes: g.nodes,
		Edges: validGraphEdges(g.nodes, g.edges),
	}, nil
}

// addCharacterNodes adds a node per character and returns each character's active
// reference asset IDs.
func (d *DB) addCharacterNodes(g *graphBuilder, projectID string) map[string]map[string]bool {
	activeReferenceIDs := map[string]map[string]bool{}
	chars, err := d.ListCharacters(projectID)
	if err == nil {
		for _, c := range chars {
			refs := models.IDList(c.ReferenceAssetIDsJSON)
			activeReferenceIDs[c.ID] = map[string]bool{}
			for _, id := range refs {
				activeReferenceIDs[c.ID][id] = true
			}
			g.nodes = append(g.nodes, models.GraphNode{
				ID:    c.ID,
				Type:  "character",
				Label: c.Name,
				Data:  map[string]interface{}{"reference_asset_ids_json": c.ReferenceAssetIDsJSON},
			})
		}
	}
	return activeReferenceIDs
}

// addAssetNodes adds a node per story asset, with reference and scene provenance edges.
func (d *DB) addAssetNodes(g *graphBuilder, projectID string, activeReferenceIDs map[string]map[string]bool) {
	assets, err := d.ListAssets(projectID)
	if err != nil {
		return
	}
	for _, a := range assets {
		// Finished edited exports are deliverables (Videos → Edited videos), not story building blocks.
		if a.Type == "output_video" {
			continue
		}
		var review struct {
			Status string `json:"consistency_review"`
		}
		json.Unmarshal(a.MetadataJSON, &review)
		g.nodes = append(g.nodes, models.GraphNode{
			ID:    a.ID,
			Type:  "asset",
			Label: a.Name,
			Data: map[string]interface{}{
				"file_path":          a.FilePath,
				"asset_type":         a.Type,
				"consistency_review": review.Status,
			},
		})
		activeReference := a.CharacterID != nil && activeReferenceIDs[*a.CharacterID][a.ID]
		if activeReference {
			g.edges = append(g.edges, models.GraphEdge{
				Source: *a.CharacterID,
				Target: a.ID,
				Label:  "reference",
			})
		}
		if len(a.MetadataJSON) > 0 {
			var meta struct {
				SceneID *string `json:"scene_id"`
			}
			if err := json.Unmarshal(a.MetadataJSON, &meta); err == nil && meta.SceneID != nil && *meta.SceneID != "" {
				g.edges = append(g.edges, models.GraphEdge{
					Source: *meta.SceneID,
					Target: a.ID,
					Label:  a.Type,
				})
			}
		}
	}
}

// addSceneNodes adds a node per scene with its casts and uses edges, followed by the
// scene's shots.
func (d *DB) addSceneNodes(g *graphBuilder, projectID string) {
	scenes, err := d.ListScenes(projectID)
	if err != nil {
		return
	}
	for _, s := range scenes {
		g.nodes = append(g.nodes, models.GraphNode{
			ID:    s.ID,
			Type:  "scene",
			Label: s.Title,
			Data: map[string]interface{}{
				"aspect_ratio": s.AspectRatio,
				"duration":     s.Duration,
				"summary":      s.Summary,
			},
		})

		// Add casts / uses edges
		charIDs := models.IDList(s.CharacterIDsJSON)
		for _, cid := range charIDs {
			g.edges = append(g.edges, models.GraphEdge{
				Source: s.ID,
				Target: cid,
				Label:  "casts",
			})
		}
		assetIDs := models.IDList(s.AssetIDsJSON)
		for _, aid := range assetIDs {
			g.edges = append(g.edges, models.GraphEdge{
				Source: s.ID,
				Target: aid,
				Label:  "uses",
			})
		}

		d.addShotNodes(g, s.ID)
	}
}

// addShotNodes adds a node per shot of the scene, linked to the scene and the assets
// the shot uses.
func (d *DB) addShotNodes(g *graphBuilder, sceneID string) {
	shots, _ := d.ListShots(sceneID)
	for _, sh := range shots {
		g.nodes = append(g.nodes, models.GraphNode{
			ID:    sh.ID,
			Type:  "shot",
			Label: fmt.Sprintf("#%d %s", sh.ShotOrder, displayPrompt(sh.Prompt)),
			Data: map[string]interface{}{
				"scene_id":   sh.SceneID,
				"shot_order": sh.ShotOrder,
				"duration":   sh.Duration,
				"prompt":     sh.Prompt,
				"camera":     sh.Camera,
				"movement":   sh.Movement,
			},
		})
		g.edges = append(g.edges, models.GraphEdge{
			Source: sceneID,
			Target: sh.ID,
			Label:  "shot",
		})

		shotAssetIDs := models.IDList(sh.AssetIDsJSON)
		for _, aid := range shotAssetIDs {
			g.edges = append(g.edges, models.GraphEdge{
				Source: sh.ID,
				Target: aid,
				Label:  "uses",
			})
		}
	}
}

// addRenderNodes adds a node per shown render job, linked to its shot or scene,
// followed by the job's outputs.
func (d *DB) addRenderNodes(g *graphBuilder, projectID string) {
	jobs, err := d.ShownRenderJobs(projectID)
	if err != nil {
		return
	}
	for _, j := range jobs {
		var request struct {
			Unit json.RawMessage `json:"render_unit"`
		}
		_ = json.Unmarshal(j.RequestJSON, &request)
		modelName := j.Model
		if idx := strings.LastIndex(modelName, "/"); idx >= 0 {
			modelName = modelName[idx+1:]
		}
		g.nodes = append(g.nodes, models.GraphNode{
			ID:    j.ID,
			Type:  "render_job",
			Label: fmt.Sprintf("%s (%s)", j.Status, modelName),
			Data: map[string]interface{}{
				"status":      j.Status,
				"render_unit": request.Unit,
				"scene_id":    j.SceneID,
				"shot_id":     j.ShotID,
			},
		})
		parentID := ""
		if j.ShotID != nil && *j.ShotID != "" {
			parentID = *j.ShotID
		} else if j.SceneID != nil && *j.SceneID != "" {
			parentID = *j.SceneID
		}
		if parentID != "" {
			g.edges = append(g.edges, models.GraphEdge{
				Source: parentID,
				Target: j.ID,
				Label:  "render",
			})
		}

		d.addOutputNodes(g, j.ID)
	}
}

// addOutputNodes adds a node per output of the render job, with its QA issues.
func (d *DB) addOutputNodes(g *graphBuilder, jobID string) {
	outputs, _ := d.ListOutputsForJob(jobID)
	for _, o := range outputs {
		qaIssues := []string{}
		if len(o.QAJSON) > 0 {
			var qaData struct {
				Issues   []string `json:"issues"`
				QAIssues []string `json:"qa_issues"`
			}
			if err := json.Unmarshal(o.QAJSON, &qaData); err == nil {
				if len(qaData.Issues) > 0 {
					qaIssues = qaData.Issues
				} else if len(qaData.QAIssues) > 0 {
					qaIssues = qaData.QAIssues
				}
			}
		}
		label := o.ID
		if o.VideoPath != "" {
			label = filepath.Base(o.VideoPath)
		}
		g.nodes = append(g.nodes, models.GraphNode{
			ID:    o.ID,
			Type:  "output",
			Label: label,
			Data: map[string]interface{}{
				"video_path":     o.VideoPath,
				"thumbnail_path": o.ThumbnailPath,
				"captioned_path": o.CaptionedPath,
				"score":          o.Score,
				"qa_issues":      qaIssues,
				"render_job_id":  o.RenderJobID,
			},
		})
		g.edges = append(g.edges, models.GraphEdge{
			Source: jobID,
			Target: o.ID,
			Label:  "output",
		})
	}
}

// validGraphEdges keeps edges whose ends are both nodes, drops duplicates, and drops
// generation provenance on a pair that also has a current usage.
func validGraphEdges(nodes []models.GraphNode, edges []models.GraphEdge) []models.GraphEdge {
	knownNodes := make(map[string]bool)
	for _, n := range nodes {
		knownNodes[n.ID] = true
	}
	validEdges := []models.GraphEdge{}
	used := map[string]bool{}
	for _, e := range edges {
		if e.Label == "uses" {
			used[e.Source+"\x00"+e.Target] = true
		}
	}
	seen := map[string]bool{}
	for _, e := range edges {
		pair := e.Source + "\x00" + e.Target
		key := pair + "\x00" + e.Label
		// A current usage supersedes generation provenance on the same pair.
		provenance := e.Label == "storyboard" || e.Label == "prop" || e.Label == "location" || e.Label == "background" || e.Label == "other"
		if knownNodes[e.Source] && knownNodes[e.Target] && !seen[key] && !(provenance && used[pair]) {
			seen[key] = true
			validEdges = append(validEdges, e)
		}
	}
	return validEdges
}
