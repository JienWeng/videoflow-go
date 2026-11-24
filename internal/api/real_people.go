package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// Fictional stand-ins for real people. Video services (Google Omni above all) refuse
// clips that name or look like a recognizable real person, and say only
// INVALID_ARGUMENT. In Full auto the assistant decides this by itself, before any
// picture or clip is paid for: each cast member who is a real, recognizable person gets
// a fictional name and a clearly different look, every text of the story is rewritten
// to the new name, and the character's picture is drawn again.
//
// The decision log keeps what was done: stand_in:<character id> holds "Old → New", and
// a character with that entry is never changed again. If the service still refuses a
// clip for safety, the check runs once more in strict mode (anyone who could be read as
// a real person is replaced) and the refused scenes are made again ONCE
// (real_people_retry:<scene id>).

// errStandInCheck is a cast check that couldn't run (the local session didn't answer).
var errStandInCheck = errors.New("couldn't check the cast for real people")

// standInAction is the chat action that replaces real people with stand-ins.
const standInAction = "replace_real_people"

func standInKey(characterID string) string     { return "stand_in:" + characterID }
func realPeopleRetryKey(sceneID string) string { return "real_people_retry:" + sceneID }

// standInPlan is the assistant's answer: the cast members to replace.
type standInPlan struct {
	StandIns []standIn `json:"stand_ins"`
}

type standIn struct {
	CharacterID string `json:"character_id"`
	RealPerson  string `json:"real_person"`
	NewName     string `json:"new_name"`
	Appearance  string `json:"appearance"`
	Description string `json:"description"`
	// Replacements map each way the story writes the person (full name, first name,
	// surname, nickname, in any language used) to the stand-in's matching form.
	Replacements []nameReplacement `json:"replacements"`
}

type nameReplacement struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// castCharacters are the characters of the run's scenes.
func (a *autopilotRunner) castCharacters() ([]models.Character, error) {
	ids := map[string]bool{}
	for _, scene := range a.scenes {
		for _, id := range models.IDList(scene.CharacterIDsJSON) {
			ids[id] = true
		}
	}
	all, err := a.s.database.ListCharacters(a.project)
	if err != nil {
		return nil, err
	}
	cast := []models.Character{}
	for _, c := range all {
		if ids[c.ID] {
			cast = append(cast, c)
		}
	}
	return cast, nil
}

// replaceRealPeople is Full auto's stand-in step. strict is the second pass after a
// refusal. It returns the stand-ins made.
func (a *autopilotRunner) replaceRealPeople(ctx context.Context, strict bool) ([]standIn, error) {
	if !a.body.FullAuto || a.body.JoinOnly {
		return nil, nil
	}
	cast, err := a.castCharacters()
	if err != nil || len(cast) == 0 {
		return nil, err
	}
	mode := standInFound
	if strict {
		mode = standInStrict
	}
	made, err := a.s.makeStandIns(ctx, a.project, cast, a.scenes, mode, a.stage, a.sameProject)
	if len(made) > 0 {
		a.reloadScenes()
	}
	return made, err
}

// Which cast members a stand-in check replaces.
const (
	standInFound  = "found"  // real, recognizable people only
	standInStrict = "strict" // anyone who could be read as a real person
	standInAll    = "all"    // every character given (the owner or the chat chose them)
)

// makeStandIns asks the local session which of cast are real people (per mode), gives
// each a fictional stand-in, rewrites the story to the new names, records it in the
// decision log and draws the new pictures. A character already replaced is skipped.
func (s *Server) makeStandIns(ctx context.Context, project string, cast []models.Character, scenes []models.Scene, mode string, stage func(string), sameProject func() error) ([]standIn, error) {
	decisions, _ := s.database.ListDecisions(project) // unreadable: nothing replaced yet
	done := map[string]bool{}
	for _, d := range decisions {
		if id, ok := strings.CutPrefix(d.Key, "stand_in:"); ok {
			done[id] = true
		}
	}
	type castFact struct {
		ID, Name, Description, Appearance string
	}
	facts := []castFact{}
	for _, c := range cast {
		if !done[c.ID] {
			facts = append(facts, castFact{c.ID, c.Name, clip(c.Description, 600), clip(c.Appearance, 600)})
		}
	}
	if len(facts) == 0 {
		return nil, nil
	}
	story := []string{}
	for _, scene := range scenes {
		story = append(story, scene.Title+": "+clip(scene.Summary, 800))
	}
	castJSON, _ := json.Marshal(facts)
	stage("Checking the cast for real people")
	rule := "List only characters who are real, recognizable people (living or historical public figures, celebrities, founders, politicians, named family members of such people). Leave out clearly fictional characters and generic roles."
	switch mode {
	case standInStrict:
		rule = "The video service refused clips of this story, most likely for showing a real person. List every character who is, is named after, or could be read as a real, recognizable person, including their relatives. When unsure, list them."
	case standInAll:
		rule = "The owner asked to replace every character listed in Cast with a fictional stand-in: list every one of them."
	}
	system := "You protect a video from being refused for depicting real people. " + rule + " For each one, invent a fictional stand-in who plays the same role in the story: a new full name that fits the story's language and setting and matches no famous person, a new appearance that is clearly different from the real person (different face shape, hair, build and clothing, same age and role), and a one-sentence description with no real names. In replacements, list every form of the real name the story or dialogue uses (full name, first name, surname, nickname, and the same names in other scripts such as Chinese), each with the stand-in's matching form; longest forms first. Keep the real person only in real_person. Return an empty stand_ins list when nobody needs replacing."
	var plan standInPlan
	if err := s.agentJSON(ctx, "story_agent", system, "Cast: "+string(castJSON)+"\nStory:\n"+strings.Join(story, "\n"), nil, &plan); err != nil {
		return nil, fmt.Errorf("%w: %v", errStandInCheck, err)
	}
	made := []standIn{}
	for _, in := range plan.StandIns {
		var c *models.Character
		for i := range cast {
			if cast[i].ID == in.CharacterID && !done[cast[i].ID] {
				c = &cast[i]
			}
		}
		if c == nil || strings.TrimSpace(in.NewName) == "" {
			continue
		}
		if err := sameProject(); err != nil {
			return made, err
		}
		stage("Replacing " + c.Name + " with the fictional " + in.NewName)
		if err := s.applyStandIn(project, c, in); err != nil {
			return made, fmt.Errorf("couldn't replace %s with a fictional stand-in: %w", c.Name, err)
		}
		if _, err := s.database.SetDecision(project, standInKey(c.ID), c.Name+" → "+in.NewName+" (video services refuse real people)", db.DecisionByAssistant, false); err != nil {
			return made, err
		}
		done[c.ID] = true
		made = append(made, in)
	}
	if len(made) > 0 {
		if err := s.redrawStandIns(ctx, made, stage); err != nil {
			return made, err
		}
	}
	return made, nil
}

// applyStandIn renames the character, gives it its new look, drops its old picture and
// rewrites every text of the project's scenes and shots to the new name.
func (s *Server) applyStandIn(project string, c *models.Character, in standIn) error {
	replace := nameReplacer(c.Name, in)
	c.Name = strings.TrimSpace(in.NewName)
	if v := strings.TrimSpace(in.Appearance); v != "" {
		c.Appearance = v
	} else {
		c.Appearance = replace(c.Appearance)
	}
	if v := strings.TrimSpace(in.Description); v != "" {
		c.Description = v
	} else {
		c.Description = replace(c.Description)
	}
	c.Personality, c.SampleDialogue = replace(c.Personality), replace(c.SampleDialogue)
	c.ReferenceAssetIDsJSON = json.RawMessage("[]") // the lookalike picture is drawn again
	if err := s.database.UpdateCharacter(c); err != nil {
		return err
	}
	scenes, err := s.database.ListScenes(project)
	if err != nil {
		return err
	}
	for i := range scenes {
		scene := &scenes[i]
		title, summary, raw := replace(scene.Title), replace(scene.Summary), replaceJSON(scene.SceneJSON, replace)
		if title != scene.Title || summary != scene.Summary || string(raw) != string(scene.SceneJSON) {
			scene.Title, scene.Summary, scene.SceneJSON = title, summary, raw
			if err = s.database.UpdateScene(scene); err != nil {
				return err
			}
		}
		shots, err := s.database.ListShots(scene.ID)
		if err != nil {
			return err
		}
		for j := range shots {
			shot := &shots[j]
			prompt, raw := replace(shot.Prompt), replaceJSON(shot.ShotJSON, replace)
			if prompt != shot.Prompt || string(raw) != string(shot.ShotJSON) {
				shot.Prompt, shot.ShotJSON = prompt, raw
				if err = s.database.UpdateShot(shot); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// nameReplacer swaps every listed form of the real name (and the character's old name)
// for the stand-in's, longest first so a full name is not half replaced.
func nameReplacer(oldName string, in standIn) func(string) string {
	pairs := append([]nameReplacement{}, in.Replacements...)
	pairs = append(pairs, nameReplacement{From: oldName, To: in.NewName})
	if in.RealPerson != "" {
		pairs = append(pairs, nameReplacement{From: in.RealPerson, To: in.NewName})
	}
	clean := []nameReplacement{}
	seen := map[string]bool{}
	for _, p := range pairs {
		p.From, p.To = strings.TrimSpace(p.From), strings.TrimSpace(p.To)
		if len([]rune(p.From)) < 2 || p.To == "" || seen[p.From] {
			continue
		}
		seen[p.From] = true
		clean = append(clean, p)
	}
	sort.SliceStable(clean, func(i, j int) bool { return len(clean[i].From) > len(clean[j].From) })
	args := []string{}
	for _, p := range clean {
		args = append(args, p.From, p.To)
	}
	r := strings.NewReplacer(args...)
	return r.Replace
}

// replaceJSON rewrites the string values of a JSON document, leaving keys and shape.
func replaceJSON(raw json.RawMessage, replace func(string) string) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case string:
			return replace(t)
		case []any:
			for i := range t {
				t[i] = walk(t[i])
			}
		case map[string]any:
			for k := range t {
				t[k] = walk(t[k])
			}
		}
		return x
	}
	out, err := json.Marshal(walk(v))
	if err != nil {
		return raw
	}
	return out
}

// redrawStandIns draws the new characters' pictures, so no lookalike reaches a clip.
func (s *Server) redrawStandIns(ctx context.Context, made []standIn, stage func(string)) error {
	chars := []models.Character{}
	for _, in := range made {
		if c, err := s.database.GetCharacter(in.CharacterID); err == nil {
			chars = append(chars, *c)
		}
	}
	if problem := s.characterSheetProblem(ctx); problem != "" {
		return fmt.Errorf("the real people were replaced with fictional stand-ins, but their new pictures can't be made: %s", problem)
	}
	stage(fmt.Sprintf("Drawing new pictures for %d fictional stand-ins", len(chars)))
	batch := s.makeCharacterSheets(ctx, chars, false, "", func(done, total int) {
		stage(fmt.Sprintf("Drawing stand-in pictures: %d of %d done", done, total))
	})
	if err := failedSheetsError(batch.Failed); err != nil {
		return fmt.Errorf("%w. The stand-ins are saved; ask me again to carry on", err)
	}
	return nil
}

// reloadScenes reads the run's scenes again after their texts changed.
func (a *autopilotRunner) reloadScenes() {
	for i := range a.scenes {
		if fresh, err := a.s.database.GetScene(a.scenes[i].ID); err == nil {
			a.scenes[i] = *fresh
		}
	}
}

// safetyRefusedScenes are the run's scenes one of whose waited-on videos was refused
// for safety (or with Google's bare INVALID_ARGUMENT) and that had no stand-in retry yet.
func (a *autopilotRunner) safetyRefusedScenes(waiting map[string][]string) []models.Scene {
	decisions, _ := a.s.database.ListDecisions(a.project)
	retried := map[string]bool{}
	for _, d := range decisions {
		if id, ok := strings.CutPrefix(d.Key, "real_people_retry:"); ok {
			retried[id] = true
		}
	}
	out := []models.Scene{}
	for _, scene := range a.scenes {
		if retried[scene.ID] {
			continue
		}
		for _, id := range waiting[scene.ID] {
			if j, err := a.s.database.GetRenderJob(id); err == nil && j.Status == models.StatusFailed && j.Error != nil && refusedForSafety(*j.Error) {
				out = append(out, scene)
				break
			}
		}
	}
	return out
}

// refusedForSafety reads a video service's failure as a refusal of what it shows.
func refusedForSafety(failure string) bool {
	text := strings.ToLower(failure)
	if strings.Contains(text, "invalid_argument") {
		return true
	}
	for _, w := range safetyWords {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// retryRefusedScenes is the one recovery after a refusal: strict stand-ins, then the
// refused scenes are made again once. It reports whether it retried anything.
func (a *autopilotRunner) retryRefusedScenes(ctx context.Context, waiting map[string][]string) (bool, any, error) {
	if !a.body.FullAuto || a.body.JoinOnly {
		return false, nil, nil
	}
	refused := a.safetyRefusedScenes(waiting)
	if len(refused) == 0 {
		return false, nil, nil
	}
	made, err := a.replaceRealPeople(ctx, true)
	if err != nil {
		return false, nil, err
	}
	if len(made) == 0 {
		return false, nil, nil // nobody to replace: the refusal is about something else
	}
	for _, scene := range refused {
		if _, err = a.s.database.SetDecision(a.project, realPeopleRetryKey(scene.ID), "Full auto replaced real people and made “"+scene.Title+"” again once", db.DecisionByAssistant, false); err != nil {
			return false, nil, err
		}
	}
	all := a.scenes
	ids := map[string]bool{}
	for _, scene := range refused {
		ids[scene.ID] = true
	}
	a.scenes = nil
	for _, scene := range all {
		if ids[scene.ID] {
			a.scenes = append(a.scenes, scene)
		}
	}
	again, err := a.startSceneVideos(ctx, nil)
	a.scenes = all
	if err != nil {
		return true, nil, err
	}
	for id, jobs := range again {
		waiting[id] = jobs
	}
	// Wait for every clip of the run: the new ones and those still being made.
	stuck, err := a.waitForSceneVideos(ctx, waiting)
	return true, stuck, err
}

// handleStandIns (POST /characters/stand-ins) replaces real people with fictional
// stand-ins outside Full auto: the chat's replace_real_people action and its button.
// Body: optional character_ids (those are all replaced); without them, every
// character of the project who could be read as a real person is. One turn of the
// local session (free) and one new picture per stand-in (picture credits).
func (s *Server) handleStandIns(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		CharacterIDs []string `json:"character_ids"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", 400)
		return
	}
	mode := standInStrict
	cast, err := s.database.ListCharacters(project)
	if len(body.CharacterIDs) > 0 {
		mode = standInAll
		cast, err = s.sheetCharacters(project, body.CharacterIDs)
	}
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if len(cast) == 0 {
		http.Error(w, "this project has no characters to replace", 409)
		return
	}
	if problem := s.characterSheetProblem(r.Context()); problem != "" {
		http.Error(w, "the new characters' pictures can't be made: "+problem, 400)
		return
	}
	scenes, _ := s.database.ListScenes(project) // the story only guides the names
	op := &models.Op{Kind: "stand_ins", ProjectID: &project}
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		stage := func(text string) {
			if op.ID != "" {
				raw, _ := json.Marshal(map[string]any{"stage": text})
				_ = s.database.UpdateOp(op.ID, models.StatusRunning, raw, nil)
			}
		}
		same := func() error {
			if active, e := s.database.GetActiveProjectID(); e != nil || active != project {
				return fmt.Errorf("the open project changed; reopen it and ask again")
			}
			return nil
		}
		made, err := s.makeStandIns(ctx, project, cast, scenes, mode, stage, same)
		replaced := []map[string]string{}
		for _, in := range made {
			replaced = append(replaced, map[string]string{"character_id": in.CharacterID, "real_person": in.RealPerson, "new_name": in.NewName})
		}
		return map[string]any{"replaced": replaced}, err
	})
}
