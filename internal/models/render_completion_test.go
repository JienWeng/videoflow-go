package models

import (
	"fmt"
	"testing"
)

func TestSceneCompletionNeedsEveryPlannedClip(t *testing.T) {
	scene := "scene"
	jobs := []RenderJob{}
	for i := 0; i < 3; i++ {
		jobs = append(jobs, RenderJob{SceneID: &scene, Status: "succeeded", RequestJSON: []byte(fmt.Sprintf(`{"render_plan_id":"plan","render_unit":{"key":"plan:%d","index":%d,"count":3}}`, i, i))})
	}
	if CompleteSceneRenders(jobs[:1])[scene] || CompleteSceneRenders([]RenderJob{jobs[0], jobs[0]})[scene] {
		t.Fatal("one subclip or alternate version marked scene complete")
	}
	if !CompleteSceneRenders(jobs)[scene] {
		t.Fatal("complete plan not recognized")
	}
	jobs[1].Status = "failed"
	if CompleteSceneRenders(jobs)[scene] {
		t.Fatal("failed clip marked scene complete")
	}
	shot := "shot"
	if CompleteSceneRenders([]RenderJob{{SceneID: &scene, ShotID: &shot, Status: "succeeded"}})[scene] {
		t.Fatal("standalone shot marked scene complete")
	}
	if !CompleteSceneRenders([]RenderJob{{SceneID: &scene, Status: "succeeded"}})[scene] {
		t.Fatal("legacy whole-scene output lost")
	}
}
