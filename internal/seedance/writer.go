package seedance

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The BytePlus prompt guides for the Seedance 2.0 and 2.5 series, kept as a playbook:
// a short index that always applies, and one section per topic. A clip's writer is
// given the index and only the sections that clip needs, so it applies the guidance in
// full without wading through parts that do not concern it.
//
//go:embed guide/*.md
var guide embed.FS

func section(name string) string {
	text, err := guide.ReadFile("guide/" + name + ".md")
	if err != nil {
		panic("seedance: missing guide section " + name)
	}
	return strings.TrimSpace(string(text))
}

// Sections names the playbook sections a clip's writer reads, in reading order.
func Sections(c Clip) []string {
	series := "2.0"
	if c.timed() {
		series = "2.5"
	}
	frame := "first-frame"
	if len(c.References) > 0 {
		frame = "references"
	}
	names := []string{"index", "bindings", frame, "shots-" + series, "action", "camera"}
	if c.Audio == Native {
		if c.speaks() && c.omni() {
			names = append(names, "speech-omni")
		} else if c.speaks() {
			names = append(names, "speech-"+series)
		}
		names = append(names, "music-"+series)
	}
	return append(names, "constraints", "example-"+series)
}

func (c Clip) speaks() bool {
	for _, l := range c.Lines {
		if strings.TrimSpace(l.Text) != "" {
			return true
		}
	}
	return false
}

// Brief is what the AI prompt writer is asked: the playbook for this clip, and the
// facts of the clip with a plain draft to improve on.
func Brief(c Clip) (system, facts string) {
	parts := []string{}
	for _, name := range Sections(c) {
		parts = append(parts, section(name))
	}
	system = strings.Join(parts, "\n\n") + "\n\n# Your task\nWrite the one prompt for the clip described in the facts. Use every shot, in order, labelled \"Shot 1\", \"Shot 2\" and so on. Improve the wording of each shot to follow the playbook (specific small actions, feelings shown through the body, one camera move, the sound of the shot), but keep what happens, who is in it and where. Write in English; spoken lines stay exactly as given, in their own language. The draft shows the minimum that must be there; your prompt should be better directed than the draft and shorter where the draft repeats itself: say the look once, describe each character once, and keep the whole prompt under 3,000 characters. Return JSON: {\"prompt\": \"...\"}."

	type line struct {
		Speaker string `json:"speaker"`
		Text    string `json:"text"`
		// At is when the line is said in the clip (Gemini Omni only), from the speech budget.
		At string `json:"at,omitempty"`
	}
	type shot struct {
		Number  int    `json:"number"`
		Seconds int    `json:"seconds,omitempty"`
		Framing string `json:"framing,omitempty"`
		Move    string `json:"camera_move,omitempty"`
		Action  string `json:"what_happens"`
		Lines   []line `json:"spoken_here,omitempty"`
	}
	speech := "nobody speaks"
	if c.Audio != Native {
		speech = "no audio at all"
	} else if c.speaks() && c.Narration {
		speech = "an unseen narrator tells it; nobody on screen speaks"
	} else if c.speaks() {
		speech = "the characters talk to each other on screen"
	}
	doc := map[string]any{
		"model": c.Family, "look": c.Style, "summary": c.Summary, "mood": c.Tone, "speech": speech, "language": c.Language,
		"music": c.Music, "cast": c.Cast, "connected_places_and_props": c.Things, "first_frame": "a storyboard picture of the opening is sent as the first frame",
		"draft": Compose(c),
	}
	if len(c.References) > 0 {
		pictures := []map[string]any{}
		for i, ref := range c.References {
			pictures = append(pictures, map[string]any{"tag": Tag(i), "shows": ref.Kind, "name": ref.Name, "reference_sheet": ref.Sheet})
		}
		delete(doc, "first_frame")
		doc["reference_pictures"] = pictures
	}
	if len(c.Voices) > 0 {
		voices := []map[string]any{}
		for i, name := range c.Voices {
			voices = append(voices, map[string]any{"tag": VoiceTag(i), "is_the_voice_of": name})
		}
		doc["voice_recordings_sent_name_each_by_its_tag"] = voices
	}
	if c.Audio == Native && c.Music == "" {
		doc["music"] = "none"
		if c.MusicInEdit {
			doc["music"] = "none in this clip: one music track is laid under the whole film in the edit, so the clip has voices, ambience and action sounds only; say there is no music"
		}
	}
	if c.Audio == Native {
		doc["ambience"] = c.ambience() + " (the scene's steady background sound: keep it the same in every shot, under the voices)"
	}
	shots, loose := []shot{}, []line{}
	for i, s := range c.Shots {
		shots = append(shots, shot{Number: i + 1, Seconds: s.Seconds, Framing: s.Framing, Move: s.Move, Action: s.Action})
	}
	for _, l := range c.Lines {
		if strings.TrimSpace(l.Text) == "" || c.Audio != Native {
			continue
		}
		who := l.Speaker
		if who == "" {
			who = "Narrator"
		}
		said := line{Speaker: who, Text: l.Text}
		if c.omni() && l.Until > l.At {
			said.At = fmt.Sprintf("%s-%ss", seconds(l.At), seconds(l.Until))
		}
		if l.Shot >= 1 && l.Shot <= len(shots) {
			shots[l.Shot-1].Lines = append(shots[l.Shot-1].Lines, said)
		} else {
			loose = append(loose, said)
		}
	}
	doc["shots"] = shots
	if len(loose) > 0 {
		doc["spoken_lines_to_place_in_the_shot_they_fit"] = loose
	}
	raw, _ := json.MarshalIndent(doc, "", " ")
	return system, string(raw)
}

// imageTag finds a mention of the n-th picture, written as the guides allow: "@Image 2",
// "@image2", "Image 2".
func imageTag(n int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?i)@?\bimage ?%d\b`, n))
}

// audioTag finds a mention of the n-th voice recording: "@Audio 1".
func audioTag(n int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?i)@audio ?%d\b`, n))
}

var internalID = regexp.MustCompile(`\b(char|asset|scene|shot|project|job|script|output)_[0-9a-f]{6,}\b`)

// Check says whether a written prompt may be sent for this clip. It guards the facts
// the writer must not change; how well the prompt is directed is the writer's job.
func Check(c Clip, prompt string) error {
	prompt = strings.TrimSpace(prompt)
	switch {
	case prompt == "":
		return fmt.Errorf("empty prompt")
	case len(prompt) > 4000:
		return fmt.Errorf("prompt is %d characters; the guides ask for concise prompts", len(prompt))
	case internalID.MatchString(prompt) || strings.Contains(prompt, `{"`):
		return fmt.Errorf("prompt contains internal ids or raw data")
	case !strings.Contains(strings.ToLower(prompt), "subtitle"):
		return fmt.Errorf("prompt does not rule out subtitles")
	}
	for i := range c.Shots {
		if !strings.Contains(prompt, fmt.Sprintf("Shot %d", i+1)) {
			return fmt.Errorf("shot %d is missing", i+1)
		}
	}
	if strings.Contains(prompt, fmt.Sprintf("Shot %d", len(c.Shots)+1)) {
		return fmt.Errorf("prompt has more shots than the scene")
	}
	for i := range c.References {
		if !imageTag(i + 1).MatchString(prompt) {
			return fmt.Errorf("reference picture %s is never named", Tag(i))
		}
	}
	if imageTag(len(c.References) + 1).MatchString(prompt) {
		return fmt.Errorf("prompt names a picture past the %d that are sent", len(c.References))
	}
	for i := range c.Voices {
		if !audioTag(i + 1).MatchString(prompt) {
			return fmt.Errorf("voice recording %s is never named", VoiceTag(i))
		}
	}
	if audioTag(len(c.Voices) + 1).MatchString(prompt) {
		return fmt.Errorf("prompt names a voice recording past the %d that are sent", len(c.Voices))
	}
	for _, l := range c.Lines {
		text := strings.TrimSpace(l.Text)
		if text == "" {
			continue
		}
		if n := strings.Count(prompt, text); c.Audio == Native && n != 1 {
			return fmt.Errorf("spoken line %q appears %d times; it must appear exactly once, word for word", text, n)
		} else if c.Audio != Native && n != 0 {
			return fmt.Errorf("a spoken line appears in a silent clip")
		}
	}
	lower := strings.ToLower(prompt)
	noMusic := strings.Contains(lower, "no bgm") || strings.Contains(lower, "no music") || strings.Contains(lower, "no background music")
	switch {
	case c.Audio != Native && !strings.Contains(lower, "no audio"):
		return fmt.Errorf("a silent clip must say there is no audio")
	case c.Audio == Native && c.Music == "" && !noMusic:
		return fmt.Errorf("no music was asked for, and the prompt does not rule it out")
	case c.Audio == Native && c.Music != "" && noMusic:
		return fmt.Errorf("music was asked for, and the prompt rules it out")
	}
	return nil
}
