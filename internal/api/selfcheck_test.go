package api

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/models"
)

// --- The reviewer's answer ---

func TestParseSelfCheckValidatesStrictly(t *testing.T) {
	good := `{"verdict":"fix","score":62,"issues":[{"code":"extra_character","severity":"high","detail":"A second girl stands behind her."}],"suggestion":"Make it again with one character.","retry":{"worth_it":true,"change":"Only one character, alone."}}`
	r := parseSelfCheck([]byte(good), selfCheckCharacterSheet)
	if r.Unavailable || r.Verdict != verdictFix || r.Score != 62 || len(r.Issues) != 1 || r.Issues[0].Code != "extra_character" || !r.Retry.WorthIt || r.Retry.Change != "Only one character, alone." {
		t.Fatalf("%+v", r)
	}
	for name, raw := range map[string]string{
		"not json":           `nope`,
		"unknown field":      `{"verdict":"pass","score":90,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""},"extra":1}`,
		"no verdict":         `{"score":90,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"unknown verdict":    `{"verdict":"great","score":90,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"score too high":     `{"verdict":"pass","score":101,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"fractional score":   `{"verdict":"pass","score":50.5,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"no score":           `{"verdict":"pass","issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"no retry":           `{"verdict":"pass","score":90,"issues":[],"suggestion":""}`,
		"bad severity":       `{"verdict":"fix","score":40,"issues":[{"code":"x","severity":"huge","detail":"d"}],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"bad code":           `{"verdict":"fix","score":40,"issues":[{"code":"Bad Code","severity":"low","detail":"d"}],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"empty detail":       `{"verdict":"fix","score":40,"issues":[{"code":"x","severity":"low","detail":" "}],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"fix without issues": `{"verdict":"fix","score":40,"issues":[],"suggestion":"","retry":{"worth_it":true,"change":"x"}}`,
		"not visible":        `{"verdict":"pass","score":0,"issues":[{"code":"not_visible","severity":"low","detail":"No image."}],"suggestion":"","retry":{"worth_it":false,"change":""}}`,
		"trailing data":      `{"verdict":"pass","score":90,"issues":[],"suggestion":"","retry":{"worth_it":false,"change":""}} {}`,
	} {
		r := parseSelfCheck([]byte(raw), selfCheckScenePicture)
		if !r.Unavailable || r.Verdict != verdictPass || !strings.HasPrefix(r.Note, selfCheckUnavailable) {
			t.Errorf("%s: invalid answer must be the unavailable pass: %+v", name, r)
		}
	}
	issues := `[` + strings.Repeat(`{"code":"x","severity":"low","detail":"d"},`, 9)
	if r := parseSelfCheck([]byte(`{"verdict":"fix","score":1,"issues":`+strings.TrimSuffix(issues, ",")+`],"suggestion":"","retry":{"worth_it":false,"change":""}}`), selfCheckClip); !r.Unavailable {
		t.Errorf("too many issues accepted: %+v", r)
	}
	if r := parseSelfCheck([]byte(`{"verdict":"fix","score":50,"issues":[{"code":"x","severity":"low","detail":"d"}],"suggestion":"","retry":{"worth_it":true,"change":"  "}}`), selfCheckScenePicture); r.Retry.WorthIt {
		t.Errorf("a retry with nothing to change is not worth it: %+v", r)
	}
}

func TestClipCheckNeverJudgesSound(t *testing.T) {
	onlySound := `{"verdict":"fail","score":20,"issues":[{"code":"lip_sync","severity":"high","detail":"Lips don't match the line."},{"code":"artifact","severity":"low","detail":"The audio is muffled."}],"suggestion":"","retry":{"worth_it":true,"change":"Better lip sync"}}`
	r := parseSelfCheck([]byte(onlySound), selfCheckClip)
	if r.Unavailable || r.Verdict != verdictPass || len(r.Issues) != 0 || r.Retry.WorthIt || r.Note != selfCheckAudioNote {
		t.Fatalf("sound-only findings must be dropped and the clip pass with the audio note: %+v", r)
	}
	mixed := `{"verdict":"fix","score":45,"issues":[{"code":"audio_mismatch","severity":"high","detail":"Wrong voice."},{"code":"missing_character","severity":"high","detail":"Mia is not in any frame."}],"suggestion":"","retry":{"worth_it":true,"change":"Show Mia in the room."}}`
	r = parseSelfCheck([]byte(mixed), selfCheckClip)
	if r.Verdict != verdictFix || len(r.Issues) != 1 || r.Issues[0].Code != "missing_character" || r.Note != selfCheckAudioNote {
		t.Fatalf("%+v", r)
	}
	// A picture keeps a finding that only mentions music in passing.
	pic := parseSelfCheck([]byte(`{"verdict":"fix","score":45,"issues":[{"code":"text_or_watermark","severity":"medium","detail":"Music notes are written as text."}],"suggestion":"","retry":{"worth_it":false,"change":""}}`), selfCheckScenePicture)
	if len(pic.Issues) != 1 {
		t.Fatalf("%+v", pic)
	}
}

func TestAspectCheckIsMeasured(t *testing.T) {
	r := selfCheckResult{Verdict: verdictPass, Score: 90, Issues: []selfCheckIssue{}}
	applyAspectCheck(&r, 1024, 1024, "9:16")
	if r.Verdict != verdictFix || len(r.Issues) != 1 || r.Issues[0].Code != "wrong_aspect" || !r.Retry.WorthIt || !strings.Contains(r.Retry.Change, "9:16") {
		t.Fatalf("%+v", r)
	}
	ok := selfCheckResult{Verdict: verdictPass, Issues: []selfCheckIssue{}}
	applyAspectCheck(&ok, 720, 1280, "9:16")
	applyAspectCheck(&ok, 0, 0, "9:16")
	applyAspectCheck(&ok, 1024, 1024, "")
	if ok.Verdict != verdictPass || len(ok.Issues) != 0 {
		t.Fatalf("%+v", ok)
	}
}

// --- A fake local session ---

// selfCheckCLI installs a fake Codex CLI that answers checks from answers (by kind, in
// order; the last answer repeats) and logs each call's arguments. It never starts a
// real CLI or reaches a service.
func selfCheckCLI(t *testing.T, s *Server, answers map[string][]string) (count func(kind string) int, args func(kind string) string) {
	t.Helper()
	dir := t.TempDir()
	raw, _ := json.Marshal(answers)
	if err := os.WriteFile(filepath.Join(dir, "answers.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "codex")
	script := fmt.Sprintf(`#!/usr/bin/env python3
import sys, json, os
prompt = sys.stdin.read()
kind = 'clip' if 'frames sampled from a generated video clip' in prompt else 'character_sheet' if 'character reference sheet' in prompt else 'scene_picture' if 'scene picture' in prompt else 'other'
log = os.path.join(%q, kind + '.log')
with open(log, 'a') as f: f.write(json.dumps(sys.argv[1:]) + '\n')
n = sum(1 for _ in open(log))
answers = json.load(open(os.path.join(%q, 'answers.json'))).get(kind) or ['{}']
text = answers[min(n, len(answers)) - 1]
print(json.dumps({'type':'thread.started','thread_id':'selfcheck-native'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':text}}))
print(json.dumps({'type':'turn.completed'}))
`, dir, dir)
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s.directorRunner.Codex = fake
	read := func(kind string) string {
		data, _ := os.ReadFile(filepath.Join(dir, kind+".log")) // no log means no call
		return string(data)
	}
	return func(kind string) int { return strings.Count(read(kind), "\n") }, read
}

func selfCheckServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a self-check must never call a paid service: %s", r.URL.Path)
	})
	project, _ := s.database.GetActiveProjectID()
	if _, err := s.database.CreateDirectorSession(project, "codex", "Checks"); err != nil {
		t.Fatal(err)
	}
	return s, project
}

func checkContext(t *testing.T, s *Server, project string) context.Context {
	t.Helper()
	sessions, _ := s.database.ListDirectorSessions(project)
	if len(sessions) == 0 {
		t.Fatal("no session")
	}
	return context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, SessionID: sessions[0].ID})
}

// writePicture saves a small PNG of the given size and shade into storage.
func writePicture(t *testing.T, s *Server, name string, w, h int, shade uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{shade, 80, 120, 255})
		}
	}
	rel := "storage/assets/" + name + ".png"
	full, _ := s.storagePath(rel)
	os.MkdirAll(filepath.Dir(full), 0700)
	file, err := os.Create(full)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return rel
}

// sheetFixture is a character with one generated reference sheet.
func sheetFixture(t *testing.T, s *Server, project string) (*models.Character, *models.Asset) {
	t.Helper()
	c := &models.Character{ProjectID: &project, Name: "Mia", Appearance: "A girl with a red scarf", Description: "A brave courier", Personality: "Curious", VisualRulesJSON: json.RawMessage(`{}`), VoiceRulesJSON: json.RawMessage(`{}`), ReferenceAssetIDsJSON: json.RawMessage(`[]`)}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	asset := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: "Mia reference", FilePath: writePicture(t, s, "mia-"+c.ID, 64, 32, 10), TagsJSON: json.RawMessage(`[]`), MetadataJSON: json.RawMessage(`{"consistency_review":"ready","locked_traits":{"scarf":"red"}}`)}
	if err := s.database.CreateAsset(asset); err != nil {
		t.Fatal(err)
	}
	c.ReferenceAssetIDsJSON, _ = json.Marshal([]string{asset.ID})
	if err := s.database.UpdateCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c, asset
}

const (
	passAnswer      = `{"verdict":"pass","score":91,"issues":[],"suggestion":"Looks right.","retry":{"worth_it":false,"change":""}}`
	extraCharAnswer = `{"verdict":"fix","score":55,"issues":[{"code":"extra_character","severity":"high","detail":"A second person stands behind Mia."}],"suggestion":"Make it again with Mia alone.","retry":{"worth_it":true,"change":"Mia alone, no other people."}}`
)

// --- Endpoints ---

func TestSelfCheckEndpointStoresAndSkipsUnchanged(t *testing.T) {
	s, project := selfCheckServer(t)
	calls, cliArgs := selfCheckCLI(t, s, map[string][]string{"character_sheet": {extraCharAnswer, passAnswer}})
	c, asset := sheetFixture(t, s, project)
	before, _ := s.database.GetAsset(asset.ID)

	w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID))
	var out struct {
		Checked bool            `json:"checked"`
		Record  selfCheckRecord `json:"self_check"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || !out.Checked || out.Record.Result.Verdict != verdictFix || out.Record.Engine != "codex" || out.Record.Hash == "" || out.Record.Attempt != 1 || calls("character_sheet") != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	saved, _ := s.database.GetAsset(asset.ID)
	if !saved.UpdatedAt.Equal(before.UpdatedAt) || !strings.Contains(string(saved.MetadataJSON), `"locked_traits"`) || storedSelfCheck(saved.MetadataJSON) == nil {
		t.Fatalf("the check must be a note, not an edit: %s", saved.MetadataJSON)
	}
	// The picture was attached to the session's turn, which ran isolated and read-only.
	if args := cliArgs("character_sheet"); strings.Count(args, "--image=") != 1 || !strings.Contains(args, "read-only") || strings.Contains(args, "resume") {
		t.Fatal(args)
	}

	// Unchanged: no second turn.
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID))
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Checked || calls("character_sheet") != 1 || out.Record.Result.Verdict != verdictFix {
		t.Fatal("an unchanged picture was checked again", w.Body.String())
	}
	g := call(t, s, "GET", "/selfcheck?kind=character_sheet&id="+asset.ID, "")
	if g.Code != 200 || !strings.Contains(g.Body.String(), `"stale":false`) || !strings.Contains(g.Body.String(), `"verdict":"fix"`) {
		t.Fatal(g.Body.String())
	}

	// A changed description is a changed artifact: checked again.
	c.Appearance = "A girl with a blue scarf"
	s.database.UpdateCharacter(c)
	if g = call(t, s, "GET", "/selfcheck?kind=storyboard&id="+asset.ID, ""); g.Code != 200 {
		t.Fatal(g.Body.String())
	}
	if g = call(t, s, "GET", "/selfcheck?kind=character_sheet&id="+asset.ID, ""); !strings.Contains(g.Body.String(), `"stale":true`) {
		t.Fatal(g.Body.String())
	}
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID))
	json.Unmarshal(w.Body.Bytes(), &out)
	if !out.Checked || out.Record.Result.Verdict != verdictPass || calls("character_sheet") != 2 {
		t.Fatal(w.Body.String())
	}
	// "Check again" forces a turn on an unchanged picture.
	w = call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q,"force":true}`, asset.ID))
	if json.Unmarshal(w.Body.Bytes(), &out); !out.Checked || calls("character_sheet") != 3 {
		t.Fatal(w.Body.String())
	}

	// Accept keeps it; bad requests are refused.
	if a := call(t, s, "POST", "/selfcheck/accept", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID)); a.Code != 200 || !strings.Contains(a.Body.String(), `"accepted_at"`) {
		t.Fatal(a.Code, a.Body.String())
	}
	for body, code := range map[string]int{`{"kind":"poem","id":"x"}`: 400, `{"kind":"clip","id":"missing"}`: 404, `{"kind":"character_sheet","id":"missing"}`: 404, `{"kind":"character_sheet"}`: 400} {
		if r := call(t, s, "POST", "/selfcheck", body); r.Code != code {
			t.Errorf("%s: %d %s", body, r.Code, r.Body.String())
		}
	}
	if r := call(t, s, "POST", "/selfcheck/accept", `{"kind":"clip","id":"missing"}`); r.Code != 404 {
		t.Error(r.Code)
	}
}

func TestSelfCheckIsUnavailableNotBlockingWithoutASession(t *testing.T) {
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("no paid calls: %s", r.URL.Path) })
	project, _ := s.database.GetActiveProjectID()
	_, asset := sheetFixture(t, s, project)
	w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID))
	var out struct {
		Record selfCheckRecord `json:"self_check"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Record.Result.Verdict != verdictPass || !out.Record.Result.Unavailable || !strings.HasPrefix(out.Record.Result.Note, selfCheckUnavailable) {
		t.Fatal(w.Code, w.Body.String())
	}
	// An unavailable result is never kept as the answer: a session added later checks it.
	project, _ = s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Checks")
	calls, _ := selfCheckCLI(t, s, map[string][]string{"character_sheet": {passAnswer}})
	call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"character_sheet","id":%q}`, asset.ID))
	if calls("character_sheet") != 1 {
		t.Fatal("an unavailable result blocked a later real check")
	}
}

func TestSelfCheckProjectCap(t *testing.T) {
	s, project := selfCheckServer(t)
	calls, _ := selfCheckCLI(t, s, map[string][]string{"character_sheet": {passAnswer}})
	_, asset := sheetFixture(t, s, project)
	old := selfCheckProjectCap
	selfCheckProjectCap = 1
	defer func() { selfCheckProjectCap = old }()
	ctx := checkContext(t, s, project)
	target, _ := s.characterSheetTarget(project, asset.ID)
	if r, _, _ := s.runSelfCheck(ctx, target, selfCheckOptions{Force: true}); r.Result.Unavailable {
		t.Fatal(r)
	}
	r, _, _ := s.runSelfCheck(ctx, target, selfCheckOptions{Force: true})
	if !r.Result.Unavailable || !strings.Contains(r.Result.Note, "checks in the last hour") || calls("character_sheet") != 1 {
		t.Fatalf("%+v", r)
	}
}

// clipFixture is a scene with a finished one-clip video.
func clipFixture(t *testing.T, s *Server, project string, c *models.Character) (*models.Scene, *models.RenderOutput) {
	t.Helper()
	cast, _ := json.Marshal([]string{c.ID})
	scene := &models.Scene{ProjectID: &project, Title: "Chase", Summary: "Mia runs across the rooftops", Duration: 4, AspectRatio: "9:16", CharacterIDsJSON: cast, SceneJSON: json.RawMessage(`{"spoken_lines":[{"id":"l1","text":"Wait for me!"}]}`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(map[string]any{"render_plan_id": "p1", "render_unit": map[string]any{"key": "p1:0", "index": 0, "count": 1, "prompt": "Mia jumps a gap"}})
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: models.StatusSucceeded, RequestJSON: request}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	video := "storage/outputs/" + job.ID + ".mp4"
	full, _ := s.storagePath(video)
	os.MkdirAll(filepath.Dir(full), 0700)
	os.WriteFile(full, []byte("not really a video"), 0600)
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: video, QAJSON: json.RawMessage(`{"speech_timing":{"video":"kept"}}`)}
	if err := s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	return scene, out
}

// fakeFrames stands in for ffmpeg: three small pictures.
func fakeFrames(t *testing.T, s *Server) func() {
	old := selfCheckFrames
	selfCheckFrames = func(ctx context.Context, video, dir string) ([]string, error) {
		frames := []string{}
		for i := 0; i < 3; i++ {
			src, _ := s.storagePath(writePicture(t, s, fmt.Sprintf("frame-%d-%d", time.Now().UnixNano(), i), 18, 32, uint8(40*i)))
			frames = append(frames, src)
		}
		return frames, nil
	}
	return func() { selfCheckFrames = old }
}

func TestClipSelfCheckStoresOnTheOutput(t *testing.T) {
	s, project := selfCheckServer(t)
	defer fakeFrames(t, s)()
	calls, cliArgs := selfCheckCLI(t, s, map[string][]string{"clip": {`{"verdict":"fix","score":48,"issues":[{"code":"lip_sync","severity":"high","detail":"Lips are off."},{"code":"missing_character","severity":"high","detail":"Mia is not in the frames."}],"suggestion":"Re-render with Mia visible.","retry":{"worth_it":true,"change":"Keep Mia in frame the whole time."}}`}})
	c, _ := sheetFixture(t, s, project)
	_, out := clipFixture(t, s, project, c)
	w := call(t, s, "POST", "/selfcheck", fmt.Sprintf(`{"kind":"clip","id":%q}`, out.ID))
	if w.Code != 200 || calls("clip") != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	saved, _ := s.database.GetRenderOutput(out.ID)
	record := storedSelfCheck(saved.QAJSON)
	if record == nil || record.Result.Verdict != verdictFix || len(record.Result.Issues) != 1 || record.Result.Issues[0].Code != "missing_character" || record.Result.Note != selfCheckAudioNote || !strings.Contains(string(saved.QAJSON), `"speech_timing"`) {
		t.Fatalf("%s", saved.QAJSON)
	}
	// Three frames and Mia's sheet were attached to the turn.
	if args := cliArgs("clip"); strings.Count(args, "--image=") != 4 || !strings.Contains(args, "--sandbox") || strings.Contains(args, "resume") {
		t.Fatal(args)
	}
}
