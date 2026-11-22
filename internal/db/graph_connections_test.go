package db_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

func TestGraphConnectionsReflectCurrentUsage(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "graph.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, err := d.CreateProject("Connections", "")
	if err != nil {
		t.Fatal(err)
	}
	scene := models.Scene{ProjectID: &p.ID, Title: "Reunion", Duration: 4, AspectRatio: "16:9"}
	if err = d.CreateScene(&scene); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"scene_id": scene.ID})
	asset := models.Asset{ProjectID: &p.ID, Name: "Bridge", Type: "location", FilePath: "bridge.png", MetadataJSON: meta}
	if err = d.CreateAsset(&asset); err != nil {
		t.Fatal(err)
	}
	scene.AssetIDsJSON, _ = json.Marshal([]string{asset.ID, asset.ID})
	if err = d.UpdateScene(&scene); err != nil {
		t.Fatal(err)
	}
	shot := models.Shot{SceneID: scene.ID, ShotOrder: 1, Duration: 4, Prompt: "Reunion"}
	if err = d.CreateShot(&shot); err != nil {
		t.Fatal(err)
	}
	g, err := d.GetGraphData(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range g.Edges {
		if e.Source == scene.ID && e.Target == asset.ID {
			count++
			if e.Label != "uses" {
				t.Fatalf("current usage mislabeled: %+v", e)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one saved usage, got %d", count)
	}
	for _, n := range g.Nodes {
		if n.ID == shot.ID && !strings.HasPrefix(n.Label, "#1 ") {
			t.Fatalf("wrong shot order: %s", n.Label)
		}
	}
	fresh, err := d.GetScene(scene.ID)
	if err != nil {
		t.Fatal(err)
	}
	scene = *fresh
	scene.AssetIDsJSON = []byte(`[]`)
	if err = d.UpdateScene(&scene); err != nil {
		t.Fatal(err)
	}
	g, err = d.GetGraphData(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range g.Edges {
		if e.Source == scene.ID && e.Target == asset.ID && e.Label != "location" {
			t.Fatalf("detached asset falsely appears selected: %+v", e)
		}
	}
}
