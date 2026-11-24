package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// aiEditFixture is a project with one finished four-second clip that says one line
// (heard at 1.0-2.5 s), a saved edit of it, a fake native engine and a fake earshot.
// Nothing real is started: no Codex or Claude CLI, no earshot, no paid service.
type aiEditFixture struct {
	s        *Server
	project  string
	script   string
	asset    string
	base     string
	dir      string // where the fakes leave what they saw
	proposal string // what the fake editor writes to out/proposal.json
}

const aiEditBase = `{"version":1,"width":160,"height":120,"fps":24,"duration":4,"caption_style":"default","clips":[{"id":"c1","asset_id":"%s","start":0,"trim":0,"duration":4,"speed":1}],"audio":[],"captions":[{"id":"cap1","utterance_id":"line-1","start":1,"end":2.5,"text":"Hello there","timing":"heard"}]}`

// aiEditGood adds a title, a quiet whoosh made with earshot before the line, and the
// large caption style, and keeps the clip and its caption as they are.
const aiEditGood = `{"summary":"A title and a soft whoosh before the first line.","media":[{"file":"whoosh.wav","kind":"sfx","made_with":"earshot","description":"Soft whoosh"}],"manifest":{"version":1,"width":160,"height":120,"fps":24,"duration":4,"caption_style":"large","clips":[{"id":"c1","asset_id":"%[1]s","start":0,"trim":0,"duration":4,"speed":1}],"audio":[{"id":"sfx1","asset_id":"new:whoosh.wav","start":0,"trim":0,"duration":0.9,"speed":1,"volume":0.3}],"texts":[{"id":"title1","text":"Hello","start":0,"end":0.9,"size":48,"color":"#ffffff","background":"transparent","font":"sans-serif","position":"center"}],"captions":[{"id":"cap1","utterance_id":"line-1","start":1,"end":2.5,"text":"Hello there","timing":"heard"}]}}`

// aiEditCut trims into the line: its first 1.5 s are gone, and with them half the line.
const aiEditCut = `{"summary":"Tighter opening.","media":[{"file":"whoosh.wav","kind":"sfx"}],"manifest":{"version":1,"width":160,"height":120,"fps":24,"duration":2.5,"caption_style":"default","clips":[{"id":"c1","asset_id":"%[1]s","start":0,"trim":1.5,"duration":2.5,"speed":1}],"audio":[{"id":"sfx1","asset_id":"new:whoosh.wav","start":0,"trim":0,"duration":0.9,"speed":1,"volume":0.3}],"captions":[{"id":"cap1","utterance_id":"line-1","start":0,"end":1,"text":"Hello there","timing":"heard"}]}}`

func newAIEditFixture(t *testing.T) *aiEditFixture {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is needed to make the test clip")
	}
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("the AI editor must not call a remote service") })
	f := &aiEditFixture{s: s, dir: t.TempDir()}
	f.project, _ = s.database.GetActiveProjectID()
	clip := filepath.Join(s.cfg.StorageRoot, "assets", "clip.mp4")
	os.MkdirAll(filepath.Dir(clip), 0755)
	if b, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x120:r=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-y", clip).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, b)
	}
	asset := &models.Asset{ProjectID: &f.project, Type: "video", Name: "Scene one", FilePath: clip}
	if err := s.database.CreateAsset(asset); err != nil {
		t.Fatal(err)
	}
	f.asset = asset.ID
	script := &models.Script{ProjectID: &f.project, Title: "Hello", Summary: "A greeting"}
	if err := s.database.CreateScript(script); err != nil {
		t.Fatal(err)
	}
	f.script = script.ID
	saved := call(t, s, "POST", "/compositions", fmt.Sprintf(aiEditBase, f.asset))
	if saved.Code != 201 {
		t.Fatalf("save base %d %s", saved.Code, saved.Body.String())
	}
	var m composition.Manifest
	json.Unmarshal(saved.Body.Bytes(), &m)
	f.base = m.Revision
	if _, err := s.database.CreateDirectorSession(f.project, "codex", "Editing"); err != nil {
		t.Fatal(err)
	}
	// earshot: makes a one-second tone wherever -o says; listens with a fixed timeline.
	earshot := filepath.Join(f.dir, "earshot")
	os.WriteFile(earshot, []byte(`#!/bin/sh
echo "$@" >> "`+f.dir+`/earshot-calls.txt"
cmd="$1"; out=""; result=""
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then out="$2"; shift; fi
  if [ "$1" = "--out" ]; then result="$2"; shift; fi
  shift
done
if [ "$cmd" = "timeline" ]; then
  printf '%s' '{"events":[{"t":1.2,"tag":"speech"}],"loudness":-18}' > "$result"
  echo "speech at 0:01, music too quiet"
  exit 0
fi
ffmpeg -v error -f lavfi -i sine=frequency=880:duration=1 -y "$out"
echo "$out: fake/tone (1.00 s)"
`), 0700)
	t.Setenv("VIDEOFLOW_EARSHOT_BIN", earshot)
	t.Setenv("OPENAI_API_KEY", "sk-leak-openai")
	t.Setenv("GEMINI_API_KEY", "leak-gemini")
	return f
}

// useEditor installs the fake editor: it records what it was started with, makes its
// sound with the earshot on its PATH and writes the proposal it was given.
func (f *aiEditFixture) useEditor(t *testing.T, proposal string) {
	t.Helper()
	f.proposal = fmt.Sprintf(proposal, f.asset)
	if err := os.WriteFile(filepath.Join(f.dir, "proposal.json"), []byte(f.proposal), 0600); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(f.dir, "codex")
	os.WriteFile(codex, []byte(`#!/bin/sh
cat > /dev/null
echo "$@" > "`+f.dir+`/args.txt"
env > "`+f.dir+`/env.txt"
cp input/context.json "`+f.dir+`/context.json"
ls .agents/skills > "`+f.dir+`/skills.txt"
earshot make "a soft whoosh" -o out/media/whoosh.wav > /dev/null || exit 3
cp "`+f.dir+`/proposal.json" out/proposal.json
printf '%s\n' '{"type":"thread.started","thread_id":"editor-1"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"summary\":\"Edited.\"}"}}' '{"type":"turn.completed"}'
`), 0700)
	f.s.directorRunner.Codex = codex
}

func (f *aiEditFixture) edit(t *testing.T, body string) (int, aiEditResult, string) {
	t.Helper()
	r := call(t, f.s, "POST", "/scripts/"+f.script+"/ai-edit", body)
	var result aiEditResult
	json.Unmarshal(r.Body.Bytes(), &result)
	return r.Code, result, r.Body.String()
}

func (f *aiEditFixture) seen(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatalf("the fake editor left no %s: %v", name, err)
	}
	return string(b)
}

func (f *aiEditFixture) aiEditAssets(t *testing.T) []models.Asset {
	t.Helper()
	assets, _ := f.s.database.ListAssets(f.project)
	found := []models.Asset{}
	for _, a := range assets {
		if a.Type == "ai_edit_media" {
			found = append(found, a)
		}
	}
	return found
}

func TestAIEditProposalIsCheckedSavedAndWaitsForTheOwner(t *testing.T) {
	f := newAIEditFixture(t)
	f.useEditor(t, aiEditGood)
	code, result, body := f.edit(t, `{"revision":"`+f.base+`"}`)
	if code != 200 || result.Status != aiEditProposed || result.Revision == "" || result.Revision == f.base || result.BaseRevision != f.base {
		t.Fatalf("%d %s", code, body)
	}
	if result.Summary != "A title and a soft whoosh before the first line." || result.Diff == nil || result.Diff.SoundsAdded != 1 || result.Diff.TitlesAdded != 1 {
		t.Fatalf("summary or diff: %s", body)
	}
	// The session was fresh, sandboxed, with its tools and context and without keys.
	args := f.seen(t, "args.txt")
	if !strings.Contains(args, "workspace-write") || !strings.Contains(args, "network_access=false") || !strings.Contains(args, "--ephemeral") || strings.Contains(args, "resume") {
		t.Fatalf("not a fresh sandboxed tool session: %s", args)
	}
	env := f.seen(t, "env.txt")
	if strings.Contains(env, "leak") {
		t.Fatal("an API key reached the editing session")
	}
	if !strings.Contains(env, "ai-edit-") || !strings.Contains(f.seen(t, "earshot-calls.txt"), "make a soft whoosh -o out/media/whoosh.wav") {
		t.Fatal("earshot was not the vetted tool on the session's PATH")
	}
	if skills := f.seen(t, "skills.txt"); !strings.Contains(skills, "videoflow-editor") || !strings.Contains(skills, "hyperframes") {
		t.Fatalf("skills: %s", skills)
	}
	var context struct {
		Lines []spokenLine `json:"spoken_lines"`
		Clips []struct {
			MediaSeconds float64 `json:"media_seconds"`
		} `json:"clips"`
		Bounds map[string]float64 `json:"length_bounds"`
		Tools  aiEditTools        `json:"tools"`
	}
	json.Unmarshal([]byte(f.seen(t, "context.json")), &context)
	if len(context.Lines) != 1 || context.Lines[0].MediaStart != 1 || context.Lines[0].MediaEnd != 2.5 || len(context.Clips) != 1 || context.Clips[0].MediaSeconds < 3.9 || context.Bounds["min"] != 2 || context.Bounds["max"] != 6 || !context.Tools.Earshot {
		t.Fatalf("context: %s", f.seen(t, "context.json"))
	}
	// Media import: the whoosh is a project asset and the proposal plays it.
	imported := f.aiEditAssets(t)
	if len(imported) != 1 || !strings.HasPrefix(imported[0].ID, "asset_aiedit_") {
		t.Fatalf("imported %+v", imported)
	}
	if _, err := os.Stat(imported[0].FilePath); err != nil {
		t.Fatal(err)
	}
	saved, err := composition.Load(f.s.compositionRoot(), f.project, result.Revision)
	if err != nil || len(saved.Audio) != 1 || saved.Audio[0].AssetID != imported[0].ID || saved.CaptionStyle != "large" || len(saved.Texts) != 1 {
		t.Fatalf("saved %+v %v", saved, err)
	}
	if err = composition.Verify(f.s.revisionDir(f.project, result.Revision), saved); err != nil {
		t.Fatal(err)
	}
	// It waits: the current edit is still the one it improves, labelled in the list.
	if f.s.currentEdit(f.project) != f.base {
		t.Fatal("a pending proposal became the current edit")
	}
	list := call(t, f.s, "GET", "/compositions", "")
	if !strings.Contains(list.Body.String(), aiEditLabel) || !strings.Contains(list.Body.String(), `"status":"proposed"`) {
		t.Fatal(list.Body.String())
	}
	// Accept makes it current; discard after it is the undo back to the edit before.
	if r := call(t, f.s, "POST", "/compositions/"+result.Revision+"/ai-edit", `{"decision":"accept"}`); r.Code != 200 || f.s.currentEdit(f.project) != result.Revision {
		t.Fatalf("accept %d %s", r.Code, r.Body.String())
	}
	time.Sleep(20 * time.Millisecond) // file times are compared
	r := call(t, f.s, "POST", "/compositions/"+result.Revision+"/ai-edit", `{"decision":"discard"}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"open_revision":"`+f.base+`"`) || f.s.currentEdit(f.project) != f.base {
		t.Fatalf("undo %d %s", r.Code, r.Body.String())
	}
}

func TestAIEditRejectsAProposalThatCutsSpeechAndImportsNothing(t *testing.T) {
	f := newAIEditFixture(t)
	f.useEditor(t, aiEditCut)
	before, _ := os.ReadDir(filepath.Join(f.s.compositionRoot(), f.project))
	code, _, body := f.edit(t, `{"revision":"`+f.base+`"}`)
	if code == 200 || !strings.Contains(body, "not kept") || !strings.Contains(body, "speech cut") || !strings.Contains(body, "Hello there") {
		t.Fatalf("%d %s", code, body)
	}
	after, _ := os.ReadDir(filepath.Join(f.s.compositionRoot(), f.project))
	if len(after) != len(before) {
		t.Fatal("a rejected proposal was saved")
	}
	if len(f.aiEditAssets(t)) != 0 {
		t.Fatal("a rejected proposal's media was imported")
	}
}

func TestAIEditCheckNamesEveryBrokenRule(t *testing.T) {
	vol := func(v float64) *float64 { return &v }
	clip := composition.Clip{ID: "c1", AssetID: "a1", Start: 0, Trim: 0, Duration: 4, Speed: 1}
	base := composition.Manifest{Version: 1, Width: 160, Height: 120, FPS: 24, Duration: 4, Clips: []composition.Clip{clip}, Captions: []composition.Cue{{ID: "cap1", UtteranceID: "line-1", Start: 1, End: 2.5, Text: "Hello there", Timing: timingHeard}}}
	allowed := map[string]bool{"a1": true, "music": true}
	media := map[string]float64{"a1": 4, "music": 30}
	cases := []struct {
		name   string
		change func(m *composition.Manifest)
		want   string
	}{
		{"kept as is", func(m *composition.Manifest) {}, ""},
		{"moved with its caption", func(m *composition.Manifest) {
			m.Duration, m.Clips[0].Start, m.Captions[0].Start, m.Captions[0].End = 5, 1, 2, 3.5
		}, ""},
		{"speech cut", func(m *composition.Manifest) { m.Clips[0].Trim, m.Clips[0].Duration = 1.5, 2.5 }, "speech cut"},
		{"speech cut by a split", func(m *composition.Manifest) {
			m.Clips = []composition.Clip{{ID: "c1", AssetID: "a1", Duration: 2, Speed: 1}, {ID: "c2", AssetID: "a1", Start: 2, Trim: 2, Duration: 2, Speed: 1}}
		}, "speech cut"},
		{"media exceeded", func(m *composition.Manifest) { m.Duration, m.Clips[0].Duration = 6, 6 }, "media exceeded"},
		{"caption off its heard time", func(m *composition.Manifest) { m.Captions[0].Start, m.Captions[0].End = 0.5, 2 }, "caption moved off the heard time"},
		{"caption removed", func(m *composition.Manifest) { m.Captions = nil }, "caption removed"},
		{"speech muted", func(m *composition.Manifest) { m.Clips[0].Volume = vol(0.2) }, "speech muted"},
		{"loud music under the line", func(m *composition.Manifest) {
			m.Audio = []composition.Clip{{ID: "m1", AssetID: "music", Duration: 4, Speed: 1, Volume: vol(0.9)}}
		}, "sound too loud under speech"},
		{"full-frame title over the speaker", func(m *composition.Manifest) {
			m.Clips = append(m.Clips, composition.Clip{ID: "t1", AssetID: "a1", Start: 1, Duration: 1, Speed: 1})
		}, "speaker hidden"},
		{"format changed", func(m *composition.Manifest) { m.Width = 320 }, "format changed"},
		{"too long", func(m *composition.Manifest) { m.Duration = 9 }, "length out of bounds"},
		{"media from elsewhere", func(m *composition.Manifest) {
			m.Audio = []composition.Clip{{ID: "x1", AssetID: "other-project", Duration: 1, Speed: 1, Volume: vol(0.1)}}
		}, "not in this project"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proposal := base
			proposal.Clips = append([]composition.Clip{}, base.Clips...)
			proposal.Captions = append([]composition.Cue{}, base.Captions...)
			c.change(&proposal)
			reasons := checkAIEditProposal(base, proposal, media, allowed)
			joined := strings.Join(reasons, "; ")
			if c.want == "" && len(reasons) > 0 || c.want != "" && !strings.Contains(joined, c.want) {
				t.Fatalf("want %q, got %q", c.want, joined)
			}
		})
	}
}

func TestFullAutoKeepsTheAIEditOnlyWhenEveryCheckPasses(t *testing.T) {
	f := newAIEditFixture(t)
	ctx := plannerContext(t, f.s)
	runner := &autopilotRunner{s: f.s, op: &models.Op{}, project: f.project, body: autopilotRequest{ScriptID: f.script, FullAuto: true}}
	f.useEditor(t, aiEditCut)
	revision, note := runner.aiEditOnce(ctx, f.base)
	if revision != f.base || note["status"] != "rejected" || !strings.Contains(fmt.Sprint(note["reasons"]), "speech cut") {
		t.Fatalf("a failing proposal was kept: %s %v", revision, note)
	}
	f.useEditor(t, aiEditGood)
	revision, note = runner.aiEditOnce(ctx, f.base)
	if revision == f.base || note["status"] != aiEditAccepted || f.s.currentEdit(f.project) != revision {
		t.Fatalf("a passing proposal was not kept: %s %v", revision, note)
	}
	if n := readAIEditNote(f.s.revisionDir(f.project, revision)); n == nil || n.DecidedBy != "full_auto" {
		t.Fatalf("note %+v", n)
	}
	// Without a local session Full auto exports the joined edit as it is.
	f.s.directorRunner.Codex = filepath.Join(f.dir, "missing-codex")
	if revision, note = runner.aiEditOnce(ctx, f.base); revision != f.base || note["status"] != "skipped" {
		t.Fatalf("%s %v", revision, note)
	}
}

func TestListenFeedsOneSelfCorrectionPassAndNeverMore(t *testing.T) {
	f := newAIEditFixture(t)
	f.useEditor(t, aiEditGood)
	_, first, body := f.edit(t, `{"revision":"`+f.base+`"}`)
	if first.Status != aiEditProposed {
		t.Fatal(body)
	}
	dir := f.s.revisionDir(f.project, first.Revision)
	// Listening needs the rendered film and the listening setting.
	if r := call(t, f.s, "POST", "/compositions/"+first.Revision+"/listen", ""); r.Code != 409 || !strings.Contains(r.Body.String(), "export") {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	clip, _ := os.ReadFile(filepath.Join(f.s.cfg.StorageRoot, "assets", "clip.mp4"))
	os.WriteFile(filepath.Join(dir, "render.mp4"), clip, 0600)
	t.Setenv("VIDEOFLOW_EARSHOT_LISTEN", "")
	if r := call(t, f.s, "POST", "/compositions/"+first.Revision+"/listen", ""); r.Code != 409 || !strings.Contains(r.Body.String(), "listening is off") {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	if r := call(t, f.s, "POST", "/scripts/"+f.script+"/ai-edit", `{"revision":"`+first.Revision+`","refine":true}`); r.Code != 409 || !strings.Contains(r.Body.String(), "listen to the exported proposal first") {
		t.Fatalf("refine before listening: %d %s", r.Code, r.Body.String())
	}
	t.Setenv("VIDEOFLOW_EARSHOT_LISTEN", "1")
	if r := call(t, f.s, "POST", "/compositions/"+first.Revision+"/listen", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "music too quiet") {
		t.Fatalf("listen %d %s", r.Code, r.Body.String())
	}
	if !strings.Contains(f.seen(t, "earshot-calls.txt"), "timeline "+filepath.Join(dir, "render.mp4")) {
		t.Fatal("earshot did not listen to the rendered film")
	}
	// The one pass gets what was heard.
	f.useEditor(t, strings.Replace(aiEditGood, `"volume":0.3`, `"volume":0.4`, 1))
	code, second, body := f.edit(t, `{"revision":"`+first.Revision+`","refine":true}`)
	if code != 200 || second.Status != aiEditProposed || second.Pass != 2 || second.BaseRevision != first.Revision {
		t.Fatalf("%d %s", code, body)
	}
	if !strings.Contains(f.seen(t, "context.json"), "music too quiet") {
		t.Fatal("the self-correction pass did not get the soundtrack description")
	}
	// Never twice (even after a pass that was not kept), and never a pass on the pass.
	if n := readAIEditNote(dir); n == nil || n.RefinedBy != second.Revision {
		t.Fatalf("the first proposal does not record its one pass: %+v", n)
	}
	if r := call(t, f.s, "POST", "/scripts/"+f.script+"/ai-edit", `{"revision":"`+first.Revision+`","refine":true}`); r.Code != 409 || !strings.Contains(r.Body.String(), "already refined once") {
		t.Fatalf("second refine: %d %s", r.Code, r.Body.String())
	}
	os.WriteFile(filepath.Join(f.s.revisionDir(f.project, second.Revision), "listen.json"), []byte(`{}`), 0600)
	if r := call(t, f.s, "POST", "/scripts/"+f.script+"/ai-edit", `{"revision":"`+second.Revision+`","refine":true}`); r.Code != 409 || !strings.Contains(r.Body.String(), "self-correction pass") {
		t.Fatalf("refine of the refine: %d %s", r.Code, r.Body.String())
	}
}

func TestAIEditSessionStopsAtItsTimeAndDiskBudgets(t *testing.T) {
	f := newAIEditFixture(t)
	slow := filepath.Join(f.dir, "codex-slow")
	os.WriteFile(slow, []byte("#!/bin/sh\ncat >/dev/null\nhead -c 200000 /dev/zero > out/big.bin\nsleep 60\n"), 0700)
	f.s.directorRunner.Codex = slow
	budget, disk, poll := aiEditBudget, aiEditDiskBudget, aiEditDiskPoll
	t.Cleanup(func() { aiEditBudget, aiEditDiskBudget, aiEditDiskPoll = budget, disk, poll })
	aiEditBudget, aiEditDiskBudget, aiEditDiskPoll = time.Minute, 100<<10, 50*time.Millisecond
	started := time.Now()
	if code, _, body := f.edit(t, `{}`); code == 200 || !strings.Contains(body, "wrote more than") || time.Since(started) > 30*time.Second {
		t.Fatalf("disk budget: %d %s", code, body)
	}
	aiEditBudget, aiEditDiskBudget = 300*time.Millisecond, 1<<30
	started = time.Now()
	if code, _, body := f.edit(t, `{}`); code == 200 || !strings.Contains(body, "ran out of its") || time.Since(started) > 30*time.Second {
		t.Fatalf("time budget: %d %s", code, body)
	}
	workspaces, _ := filepath.Glob(filepath.Join(filepath.Dir(f.s.cfg.DatabasePath), ".videoflow-agent-workspaces", f.project, "ai-edit-*"))
	if len(workspaces) != 0 {
		t.Fatalf("workspaces left behind: %v", workspaces)
	}
}

func TestAIEditNeedsAStoryOfThisProjectAndALocalSession(t *testing.T) {
	f := newAIEditFixture(t)
	if code, _, _ := f.edit(t, `{"revision":"../x"}`); code != 400 {
		t.Fatal(code)
	}
	if r := call(t, f.s, "POST", "/scripts/script_missing/ai-edit", `{}`); r.Code != 404 {
		t.Fatal(r.Code)
	}
	f.s.directorRunner.Codex = filepath.Join(f.dir, "missing-codex")
	if code, _, body := f.edit(t, `{}`); code != 409 || !strings.Contains(body, "not installed") {
		t.Fatalf("%d %s", code, body)
	}
}
