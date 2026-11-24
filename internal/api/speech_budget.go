package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// The speech budget says, before any video is paid for, whether a scene's spoken lines
// can be said in the shots and clips they are spoken in. Each line belongs to a shot:
// its own shot_id, else the clip chosen for it in the video options, else the shot that
// is on screen when the line before it ends (a guess shown here and never saved).

// Where a line's shot came from.
const (
	placedLine  = "line"  // the line's own shot_id
	placedClip  = "clip"  // the clip chosen in the video options
	placedAuto  = "auto"  // by time: the shot on screen when the line before it ends
	placedScene = "scene" // the scene has no shots yet
)

type speechLine struct {
	ID          string   `json:"id"`
	CharacterID string   `json:"character_id"`
	Speaker     string   `json:"speaker"`
	Language    string   `json:"language"`
	Text        string   `json:"text"`
	ShotID      string   `json:"shot_id"`
	Placed      string   `json:"placed"`
	StartHint   *float64 `json:"start_hint,omitempty"`
	// Seconds, Start and End are the estimate; Start and End count from the shot's start.
	Seconds float64 `json:"seconds"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Verdict string  `json:"verdict"`
	// lead is when a line placed by time starts in its shot: after the line before it
	// (which may run on from the shot before) and a breath.
	lead float64
}

type shotSpeech struct {
	ShotID  string        `json:"shot_id"`
	Number  int           `json:"number"`
	Start   float64       `json:"start"` // from the start of the scene
	Seconds float64       `json:"seconds"`
	Used    float64       `json:"used"`
	Free    float64       `json:"free"`
	Verdict string        `json:"verdict"`
	Budget  speech.Budget `json:"budget"`
	Lines   []speechLine  `json:"lines"`
}

type clipSpeech struct {
	Index   int      `json:"index"`
	ShotIDs []string `json:"shot_ids"`
	Seconds float64  `json:"seconds"`
	Used    float64  `json:"used"`
	Free    float64  `json:"free"`
	Verdict string   `json:"verdict"`
	// ShotsOver counts shots whose own lines run past the shot's end.
	ShotsOver int    `json:"shots_over"`
	Summary   string `json:"summary"`
	// SpeechSeconds is the time the words themselves take (Used adds the pauses);
	// BreathingRoom is the time the clip keeps without speech for its ambience and the
	// music bed (speech.MusicRoom), and RoomForMusic whether Free leaves it.
	SpeechSeconds float64 `json:"speech_seconds"`
	BreathingRoom float64 `json:"breathing_room"`
	RoomForMusic  bool    `json:"room_for_music"`
	// Room says it in words: "6.1 s of speech + 1 s for music and ambience = 7.1 of 8 s".
	Room string `json:"room,omitempty"`
}

// sceneSpeech is a scene's speech budget: per shot (for writing) and per clip (what the
// video model is asked to say in one generation).
type sceneSpeech struct {
	SceneID  string       `json:"scene_id"`
	Title    string       `json:"title"`
	Language string       `json:"language"`
	Lines    int          `json:"lines"`
	Verdict  string       `json:"verdict"`
	Summary  string       `json:"summary"`
	Shots    []shotSpeech `json:"shots"`
	Clips    []clipSpeech `json:"clips"`
}

// worst is the more serious of two verdicts.
func worstVerdict(a, b string) string {
	rank := map[string]int{"": 0, speech.OK: 1, speech.Tight: 2, speech.TooLong: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// seconds1 writes seconds with one decimal and no trailing ".0".
func seconds1(v float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") + " s"
}

// speechSummary is the line the confirm card and the stages show for one clip.
func speechSummary(used, seconds float64, verdict string, shotsOver int) string {
	switch {
	case verdict == speech.TooLong:
		return fmt.Sprintf("Won't fit: %s of %s — fit the script first", seconds1(used), seconds1(seconds))
	case shotsOver > 0:
		return fmt.Sprintf("Speech fits the clip (%s of %s), but a line runs past its shot", seconds1(used), seconds1(seconds))
	case verdict == speech.Tight:
		return fmt.Sprintf("Speech fits, just: %s of %s", seconds1(used), seconds1(seconds))
	}
	return fmt.Sprintf("Speech fits: %s of %s ✓", seconds1(used), seconds1(seconds))
}

// speechCalibration is the measured speaking rates saved in settings ("speech_rates":
// {"zh": {"chars_per_second": 4.6}}); empty until something measures them.
func (s *Server) speechCalibration() speech.Calibration {
	cal := speech.Calibration{}
	if raw, err := s.database.GetAppSetting("speech_rates"); err == nil {
		_ = json.Unmarshal([]byte(raw), &cal) // unreadable calibration keeps the built-in rates
	}
	return cal
}

// planSpeech is the speech budget of lines in a scene's shots and clips. units are the
// clips the scene is made in; with none, the whole scene is one clip.
func planSpeech(scene *models.Scene, lines []SpokenLine, shots []models.Shot, units []renderUnit, names map[string]string, cal speech.Calibration) sceneSpeech {
	out := sceneSpeech{SceneID: scene.ID, Title: scene.Title, Language: sceneBrief(scene).Language, Shots: []shotSpeech{}, Clips: []clipSpeech{}}
	shots = append([]models.Shot{}, shots...)
	sort.SliceStable(shots, func(i, j int) bool { return shots[i].ShotOrder < shots[j].ShotOrder })
	at := 0.0
	index := map[string]int{}
	for i, sh := range shots {
		out.Shots = append(out.Shots, shotSpeech{ShotID: sh.ID, Number: i + 1, Start: at, Seconds: float64(sh.Duration), Lines: []speechLine{}})
		index[sh.ID] = i
		at += float64(sh.Duration)
	}
	whole := len(out.Shots) == 0
	if whole {
		out.Shots = append(out.Shots, shotSpeech{Number: 0, Seconds: float64(scene.Duration), Lines: []speechLine{}})
	}
	clipChoice := sceneProduction(scene).DialogueShots

	// Place each line, in script order. used[i] is how far into shot i its lines reach.
	used := make([]float64, len(out.Shots))
	cursor := 0.0 // scene time where the last placed line ends
	for i, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		id := l.ID
		if id == "" {
			id = fmt.Sprintf("line-%d", i)
		}
		language := l.Language
		if len(language) <= 3 && out.Language != "" && speech.Code(language) == speech.Code(out.Language) {
			language = out.Language
		}
		if language == "" {
			language = out.Language
		}
		line := speechLine{ID: id, CharacterID: l.CharacterID, Speaker: names[l.CharacterID], Language: language, Text: strings.TrimSpace(l.Text), StartHint: l.StartHint}
		if line.Speaker == "" {
			line.Speaker = "Narrator"
		}
		line.Seconds = speech.Round(speech.Seconds(line.Text, language, cal))
		shot := -1
		switch k, ok := index[l.ShotID]; {
		case whole:
			shot, line.Placed = 0, placedScene
		case ok && l.ShotID != "":
			shot, line.Placed = k, placedLine
		default:
			if k, ok := index[clipChoice[id]]; ok {
				shot, line.Placed = k, placedClip
			}
		}
		if shot < 0 {
			// The shot on screen when this line would start; when less than half of
			// the line fits in what is left of that shot, the next one.
			line.Placed, shot = placedAuto, len(out.Shots)-1
			for k, sh := range out.Shots {
				start := max(cursor, sh.Start+used[k])
				if start < sh.Start+sh.Seconds && (sh.Start+sh.Seconds-start >= line.Seconds/2 || k == len(out.Shots)-1) {
					shot = k
					break
				}
			}
		}
		line.ShotID = out.Shots[shot].ShotID
		begin := used[shot]
		if used[shot] > 0 {
			begin += speech.Pause
		}
		if line.Placed == placedAuto && out.Lines > 0 {
			if carried := cursor + speech.Pause - out.Shots[shot].Start; carried > begin {
				begin, line.lead = carried, carried
			}
		}
		out.Shots[shot].Lines = append(out.Shots[shot].Lines, line)
		used[shot] = begin + line.Seconds
		cursor = out.Shots[shot].Start + used[shot]
		out.Lines++
	}

	for k := range out.Shots {
		sh := &out.Shots[k]
		language := out.Language
		fitIn := []speech.Line{}
		for _, l := range sh.Lines {
			fitIn = append(fitIn, speech.Line{ID: l.ID, Text: l.Text, Language: l.Language, Hint: l.hint()})
			language = l.Language
		}
		f := speech.FitLines(fitIn, sh.Seconds, cal)
		sh.Used, sh.Free, sh.Verdict = f.Used, f.Free, f.Verdict
		if len(sh.Lines) == 0 {
			sh.Verdict = ""
		}
		for i := range sh.Lines {
			sh.Lines[i].Start, sh.Lines[i].End, sh.Lines[i].Verdict = f.Lines[i].Start, f.Lines[i].End, f.Lines[i].Verdict
		}
		sh.Budget = speech.BudgetFor(sh.Seconds, language, cal)
	}

	// Clips: the lines of a clip's shots spoken one after another, each shot's lines
	// waiting for their shot, so a shot that runs over pushes the next one's lines later.
	if len(units) == 0 {
		ids := []string{}
		for _, sh := range out.Shots {
			if sh.ShotID != "" {
				ids = append(ids, sh.ShotID)
			}
		}
		units = []renderUnit{{ShotIDs: ids, Length: int(at)}}
		if whole {
			units[0].Length = scene.Duration
		}
	}
	for i, u := range units {
		c := clipSpeech{Index: i, ShotIDs: u.ShotIDs, Seconds: float64(u.Length)}
		fitIn, offset := []speech.Line{}, 0.0
		members := u.ShotIDs
		if whole {
			members = []string{""}
		}
		for _, id := range members {
			k, ok := index[id]
			if whole {
				k, ok = 0, true
			}
			if !ok {
				continue
			}
			sh := out.Shots[k]
			for _, l := range sh.Lines {
				fitIn = append(fitIn, speech.Line{ID: l.ID, Text: l.Text, Language: l.Language, Hint: offset + max(0, l.hint())})
			}
			if sh.Verdict == speech.TooLong {
				c.ShotsOver++
			}
			offset += sh.Seconds
		}
		if c.Seconds <= 0 {
			c.Seconds = offset
		}
		if len(fitIn) == 0 {
			out.Clips = append(out.Clips, c)
			continue
		}
		f := speech.FitLines(fitIn, c.Seconds, cal)
		c.Used, c.Free, c.Verdict = f.Used, f.Free, f.Verdict
		for _, l := range f.Lines {
			c.SpeechSeconds += l.Seconds
		}
		c.SpeechSeconds = speech.Round(c.SpeechSeconds)
		c.BreathingRoom = speech.MusicRoom(c.Seconds)
		c.RoomForMusic = c.Free+1e-9 >= c.BreathingRoom
		c.Room = fmt.Sprintf("%s of speech + %s for music and ambience = %s of %s", seconds1(c.Used), seconds1(c.BreathingRoom), seconds1(speech.Round(c.Used+c.BreathingRoom)), seconds1(c.Seconds))
		if !c.RoomForMusic {
			c.Room += " (no moment without speech)"
		}
		if c.ShotsOver > 0 {
			c.Verdict = worstVerdict(c.Verdict, speech.Tight)
		}
		c.Summary = speechSummary(c.Used, c.Seconds, c.Verdict, c.ShotsOver)
		if len(units) > 1 {
			c.Summary = fmt.Sprintf("Clip %d · %s", i+1, c.Summary)
		}
		out.Clips = append(out.Clips, c)
		out.Verdict = worstVerdict(out.Verdict, c.Verdict)
	}
	for _, c := range out.Clips {
		if c.Verdict == out.Verdict && c.Summary != "" {
			out.Summary = c.Summary
			break
		}
	}
	return out
}

// hint is when a line starts in its shot: its saved start hint, else when the line
// before it ends, else -1 (it follows the line before it in the same shot).
func (l speechLine) hint() float64 {
	switch {
	case l.StartHint != nil:
		return *l.StartHint
	case l.lead > 0:
		return l.lead
	}
	return -1
}

// speakerNames are the names of a project's characters by id.
func (s *Server) speakerNames(project string) map[string]string {
	names := map[string]string{}
	chars, _ := s.database.ListCharacters(project) // unreadable characters show as Narrator-less ids
	for _, c := range chars {
		names[c.ID] = c.Name
	}
	return names
}

// sceneSpeechFor is a scene's speech budget for lines (its saved lines when nil), in the
// clips its production plan makes when the plan can be worked out, else as one clip.
func (s *Server) sceneSpeechFor(ctx context.Context, scene *models.Scene, lines []SpokenLine) sceneSpeech {
	if lines == nil {
		lines = sceneSpokenLines(scene)
	}
	shots, _ := s.database.ListShots(scene.ID) // a scene whose shots cannot be read is fitted as a whole
	var units []renderUnit
	if len(shots) > 0 {
		if plan, err := s.sceneProductionPlan(ctx, scene); err == nil {
			if plan.Speech != nil && sameLines(lines, sceneSpokenLines(scene)) {
				return *plan.Speech
			}
			units = plan.Units
		}
	}
	project := ""
	if scene.ProjectID != nil {
		project = *scene.ProjectID
	}
	return planSpeech(scene, lines, shots, units, s.speakerNames(project), s.speechCalibration())
}

// handleSceneSpeechFit is GET (the saved lines) and POST (lines being edited, not saved)
// /scenes/{id}/speech-fit.
func (s *Server) handleSceneSpeechFit(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, sceneErr := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || sceneErr != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found in active project", http.StatusNotFound)
		return
	}
	var lines []SpokenLine
	if r.Method == http.MethodPost {
		var body struct {
			Lines []SpokenLine `json:"lines"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&body) != nil || body.Lines == nil || len(body.Lines) > maxSpokenLines {
			http.Error(w, "invalid spoken script", http.StatusBadRequest)
			return
		}
		lines = body.Lines
	}
	writeJSON(w, http.StatusOK, s.sceneSpeechFor(r.Context(), scene, lines))
}

// sameLines says whether two scripts are the same lines.
func sameLines(a, b []SpokenLine) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// overrun is how far over its most overfull clip is: "11.4 s of 8 s", or "clip 2: 6.1 s
// of 5 s" when the scene is made in several clips. Empty when every clip has room.
func (p sceneSpeech) overrun() string {
	worst, at := 0.0, -1
	for i, c := range p.Clips {
		if c.Verdict == speech.TooLong && (at < 0 || c.Used-c.Seconds > worst) {
			worst, at = c.Used-c.Seconds, i
		}
	}
	if at < 0 {
		return ""
	}
	c := p.Clips[at]
	text := seconds1(c.Used) + " of " + seconds1(c.Seconds)
	if len(p.Clips) > 1 {
		text = fmt.Sprintf("clip %d: %s", at+1, text)
	}
	return text
}
