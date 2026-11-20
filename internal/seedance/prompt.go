// Package seedance writes the text prompt for one Seedance clip the way BytePlus's
// prompt guides for the 2.0 and 2.5 series ask for it: a one-sentence summary, the cast
// in a few stable features, the shots in order with one camera move each, each spoken
// line written once with how it is delivered, the music, and a short list of constraints.
// Nothing else goes in: no ids, no raw records, no repeated style text.
package seedance

import (
	"fmt"
	"strings"
)

// ShotRules is the part of the guides a writer of single shots needs, and no more. It is
// given to the AI that plans a scene's shots.
const ShotRules = `Write each shot for the Seedance video model:
- prompt: what is seen, in this order: who (by name, as already described), doing what, where, light and colour. Describe the scene's look once in shot 1 and do not repeat the style or the frame shape in later shots.
- Actions are specific and small: name the body part and the speed or force ("slowly raises his right hand", "turns her head a little"). Prefer slow, gentle, continuous movement; avoid sprints, jumps and violent motion. Say how one action flows into the next.
- Show feelings through the body, not adjectives: "shoulders drop, eyes redden, fingers grip the railing" instead of "very sad". Keep emotional words mild ("amazed", not "extremely shocked").
- camera: shot size and angle only (wide shot, medium shot, close-up, low angle, over-the-shoulder).
- movement: exactly ONE camera move per shot in standard terms (slow push-in, pull back, pan left, tracking shot, orbit, fixed camera, handheld). Never combine moves.
- Do not write spoken lines, subtitles, captions or on-screen text into the prompt, and do not mention music. Speech and music are added separately.
- Use positive descriptions; do not list things to avoid. Give each shot enough to fill its seconds, and no more.`

// Shot is one planned shot of the clip.
type Shot struct {
	Seconds int
	Framing string // shot size and angle
	Move    string // the one camera move
	Action  string // what is seen
}

// Line is one spoken line. Speaker is empty for an unseen narrator.
type Line struct {
	Speaker  string
	Text     string
	Language string
	Shot     int // the shot it is spoken over, counted from 1; 0 when not pinned to one
}

// Character is a member of the cast as the video model should see them.
type Character struct {
	Name   string `json:"name"`
	Look   string `json:"look"`
	Voice  string `json:"voice,omitempty"`  // how they sound, when known
	Manner string `json:"manner,omitempty"` // how they carry themselves and speak
}

// Thing is a place or prop connected to the scene.
type Thing struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Look string `json:"look"`
}

// Audio modes.
const (
	Native = "native" // the video model makes voices, sound and music
	Silent = "silent" // picture only
)

// Clip is everything one prompt is written from.
type Clip struct {
	Family    string // "seedance-2.0", "seedance-2.5-flash", ...
	Style     string // the look, said once
	Summary   string // who, where, what happens
	Tone      string // mood of the story, used for how voices sound
	Narration bool   // an unseen narrator tells it; otherwise characters talk
	Cast      []Character
	Things    []Thing // places and props connected to the scene
	Shots     []Shot
	Lines     []Line
	Language  string
	Audio     string
	Music     string // music direction; empty means no music
}

func (c Clip) timed() bool { return strings.Contains(c.Family, "2.5") }

// Compose writes the prompt.
func Compose(c Clip) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("%s", sentence(join(". ", c.Style, c.Summary)))
	used := strings.ToLower(c.Summary)
	for _, shot := range c.Shots {
		used += " " + strings.ToLower(shot.Action)
	}
	for _, l := range c.Lines {
		used += " " + strings.ToLower(l.Speaker)
	}
	for _, person := range c.Cast {
		if person.Name != "" && person.Look != "" && strings.Contains(used, strings.ToLower(person.Name)) {
			line("%s: %s", person.Name, sentence(brief(person.Look, 260)))
		}
	}

	at := 0
	for i, shot := range c.Shots {
		head := fmt.Sprintf("Shot %d", i+1)
		if c.timed() && shot.Seconds > 0 {
			// The 2.5 series follows second-by-second timelines; 2.0 paces shots better itself.
			head = fmt.Sprintf("Shot %d (%d-%ds)", i+1, at, at+shot.Seconds)
		}
		at += shot.Seconds
		line("%s: %s", head, sentence(join(". ", join(", ", shot.Framing, shot.Move), shot.Action)))
		if c.Audio == Native {
			c.say(line, i+1)
		}
	}

	switch {
	case c.Audio != Native:
		line("No audio.")
	default:
		c.speech(line)
		c.music(line)
	}
	line("No subtitles, no captions, no on-screen text, no logo, no watermark. Natural, smooth, continuous motion; faces and bodies stay consistent.")
	return strings.TrimSpace(b.String())
}

// say writes the lines spoken over one shot (or, for shot 0, the lines pinned to none),
// each once, with who says it and how. A line already written into a shot is left there:
// saying a line twice makes the model draw subtitles.
func (c Clip) say(line func(string, ...any), shot int) {
	shown := ""
	for _, s := range c.Shots {
		shown += " " + s.Action
	}
	voices := map[string]string{}
	for _, person := range c.Cast {
		voices[person.Name] = person.Voice
	}
	for _, l := range c.Lines {
		text := strings.TrimSpace(l.Text)
		if pinned := l.Shot >= 1 && l.Shot <= len(c.Shots); text == "" || strings.Contains(shown, text) || (pinned && l.Shot != shot) || (!pinned && shot != 0) {
			continue
		}
		language := l.Language
		if language == "" {
			language = c.Language
		}
		who, how := l.Speaker, voices[l.Speaker]
		if who == "" {
			who, how = "Narrator", ""
		}
		how = join(", ", how, strings.ToLower(c.Tone))
		switch {
		case c.timed() && how != "":
			line("%s's line (%s): %q", who, how, text)
		case c.timed():
			line("%s's line: %q", who, text)
		case l.Speaker == "":
			line("Voice-over%s: {%s}", marked(language), text)
		default:
			line("%s says%s: {%s}", who, marked(language), text)
		}
	}
}

// speech writes the lines not pinned to a shot, then how the voices should sound.
func (c Clip) speech(line func(string, ...any)) {
	c.say(line, 0)
	language, spoken := c.Language, 0
	for _, l := range c.Lines {
		if strings.TrimSpace(l.Text) != "" {
			spoken++
			if l.Language != "" {
				language = l.Language
			}
		}
	}
	if spoken == 0 {
		line("Nobody speaks. Natural ambient sound and action sounds only.")
		return
	}
	in := ""
	if language != "" {
		in = " in " + language
	}
	if c.Narration {
		line("The voice is one unseen narrator, a warm, natural, expressive human voice%s, telling this like a documentary: unhurried, with real feeling, natural breaths and a short pause between sentences. Nobody on screen moves their lips. Each line is spoken once and the last one ends before the final second.", in)
	} else {
		line("Each character speaks%s in their own natural, expressive human voice, at a relaxed conversational pace with small pauses, and their lips match the words. Each line is spoken once and the last one ends before the final second.", in)
	}
	line("Natural ambient sound fits each shot and stays under the voice.")
}

func (c Clip) music(line func(string, ...any)) {
	music := strings.TrimRight(strings.TrimSpace(c.Music), ".")
	switch {
	case music != "" && c.timed():
		line("Background music: %s, quiet under the voice from start to end, fading out at the end.", music)
	case music != "":
		line("(%s plays quietly in the background under the voice, fading out at the end)", music)
	case c.timed():
		line("No music of any kind: no background music, BGM, score, instrumental, melody or ambient pad.")
	default:
		line("No BGM.")
	}
}

// marked names the language for the 2.0 series when it is neither Chinese nor English,
// as its guide requires.
func marked(language string) string {
	switch l := strings.ToLower(language); {
	case l == "", strings.Contains(l, "english"), strings.Contains(l, "chinese"), strings.Contains(l, "mandarin"), strings.Contains(l, "cantonese"):
		return ""
	}
	return " in " + language
}

func join(sep string, parts ...string) string {
	kept := []string{}
	for _, part := range parts {
		if part = strings.TrimRight(strings.TrimSpace(part), ".,; "); part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}

func sentence(text string) string {
	if text = strings.Join(strings.Fields(text), " "); text == "" || strings.ContainsAny(text[len(text)-1:], ".!?\"”。！？") {
		return text
	}
	return text + "."
}

// brief keeps the leading whole sentences of a description within limit characters.
func brief(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	cut := strings.LastIndex(text[:limit], ". ")
	if cut < limit/3 {
		cut = strings.LastIndex(text[:limit], " ")
	}
	return text[:cut]
}
