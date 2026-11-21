package api

import (
	"context"
	"encoding/json"
	"testing"
	"videoflow-go/internal/models"
)

func TestProductionShotGrouping(t *testing.T) {
	shots := []models.Shot{{ID: "a", ShotOrder: 1, Duration: 3}, {ID: "b", ShotOrder: 2, Duration: 2}, {ID: "c", ShotOrder: 3, Duration: 3}}
	durations := []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	units, e := groupRenderUnits(shots, 8, "auto", durations)
	if e != nil || len(units) != 1 || units[0].Duration != 8 || units[0].Length != 8 {
		t.Fatal(units, e)
	}
	units, e = groupRenderUnits(shots, 8, "shots", durations)
	if e != nil || len(units) != 3 {
		t.Fatal(units, e)
	}
	total := 0
	for _, u := range units {
		total += u.Duration
	}
	if total != 12 {
		t.Fatal("minimum-duration overhead hidden", units)
	}
	shots = append(shots, models.Shot{ID: "d", ShotOrder: 4, Duration: 8})
	units, e = groupRenderUnits(shots, 16, "auto", durations)
	if e != nil || len(units) != 2 || units[0].Length != 8 || units[1].Start != 8 {
		t.Fatal(units, e)
	}
	economical, e := groupRenderUnits([]models.Shot{{Duration: 7}, {Duration: 8}, {Duration: 1}}, 16, "auto", durations)
	if e != nil || len(economical) != 2 || economical[0].Duration+economical[1].Duration != 16 {
		t.Fatal("automatic grouping wasted minimum-duration padding", economical, e)
	}
	shots[0].Duration = 16
	if _, e = groupRenderUnits(shots, 29, "auto", durations); e == nil {
		t.Fatal("oversized shot silently split")
	}
	if _, e = groupRenderUnits(nil, 8, "auto", durations); e == nil {
		t.Fatal("missing shots accepted")
	}
	if _, e = groupRenderUnits([]models.Shot{{Duration: 3}}, 8, "auto", durations); e == nil {
		t.Fatal("inconsistent timeline accepted")
	}
}

func TestNativeAudioDoesNotRequireSavedVoices(t *testing.T) {
	s := voiceFixture(t, nil)
	s.database.SetAppSetting("default_video_provider", `"fal"`)
	s.database.SetAppSetting("video_model", `"bytedance/seedance-2.0/fast/image-to-video"`)
	project, _ := s.database.GetActiveProjectID()
	c := voiceCharacter(t, s)
	scene := &models.Scene{ProjectID: &project, Title: "Pip", Duration: 8, AspectRatio: "9:16", CharacterIDsJSON: voiceJSON([]string{c}), SceneJSON: voiceJSON(map[string]any{"spoken_lines": []SpokenLine{{CharacterID: c, Text: "Mama, I found you!", Language: "en"}}})}
	s.database.CreateScene(scene)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, RequestJSON: []byte(`{"prompt":"Reunion","audio_mode":"native","music_prompt":"gentle piano"}`)}
	if e := s.applyDirectorVoicePolicy(context.Background(), job); e != nil {
		t.Fatal(e)
	}
	var p map[string]any
	json.Unmarshal(job.RequestJSON, &p)
	if p["generate_audio"] != true {
		t.Fatal("native audio disabled", string(job.RequestJSON))
	}
	if p["voice_policy"].(map[string]any)["mode"] != "native" {
		t.Fatal(p)
	}
}
