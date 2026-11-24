package seedance

import (
	"strings"
	"testing"
)

func narrated(family string) Clip {
	return Clip{
		Family: family, Style: "Photorealistic live-action documentary", Summary: "A narrator tells how humanity ended quietly while empty streets hum with machines",
		Tone: "Calm, sorrowful", Narration: true, Language: "Mandarin", Audio: Native,
		Cast: []Character{{Name: "Chen Wei", Look: "East Asian man in his late thirties with short black hair. Wears an olive work jacket."}, {Name: "Mira", Look: "Not in this clip."}},
		Shots: []Shot{
			{Seconds: 4, Framing: "Wide shot at street level", Move: "slow push-in", Action: "An empty avenue at grey dawn; small delivery robots glide past"},
			{Seconds: 6, Framing: "Medium shot", Move: "fixed camera", Action: "Chen Wei slowly turns his head toward the skyline"},
		},
		Lines: []Line{{Text: "人类的终结，没有爆炸。"}, {Text: "我们把选择交给机器。"}},
	}
}

func TestEachLineIsWrittenOnceAndTheVoiceIsDescribed(t *testing.T) {
	for _, family := range []string{"seedance-2.0", "seedance-2.5"} {
		prompt := Compose(narrated(family))
		for _, text := range []string{"人类的终结，没有爆炸。", "我们把选择交给机器。"} {
			if strings.Count(prompt, text) != 1 {
				t.Errorf("%s: line %q appears %d times:\n%s", family, text, strings.Count(prompt, text), prompt)
			}
		}
		for _, want := range []string{"natural, expressive human voice in Mandarin", "No subtitles", "Chen Wei: East Asian man"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s: prompt lacks %q:\n%s", family, want, prompt)
			}
		}
		if strings.Contains(prompt, "Mira") {
			t.Errorf("%s: a character who is not in the clip is described", family)
		}
		if len(prompt) > 1600 {
			t.Errorf("%s: prompt is %d characters", family, len(prompt))
		}
	}
}

func TestEachSeriesGetsItsOwnNotation(t *testing.T) {
	two, twoFive := Compose(narrated("seedance-2.0-fast")), Compose(narrated("seedance-2.5-flash"))
	if !strings.Contains(two, "Shot 1: Wide shot") || !strings.Contains(two, "Voice-over: {人类的终结，没有爆炸。}") || !strings.Contains(two, "No BGM.") {
		t.Errorf("2.0 notation:\n%s", two)
	}
	if !strings.Contains(twoFive, "Shot 2 (4-10s): Medium shot") || !strings.Contains(twoFive, `Narrator's line (calm, sorrowful): "人类的终结，没有爆炸。"`) || !strings.Contains(twoFive, "no background music, BGM, score") {
		t.Errorf("2.5 notation:\n%s", twoFive)
	}
}

func TestALineAlreadyInAShotIsNotRepeated(t *testing.T) {
	clip := narrated("seedance-2.0")
	clip.Shots[0].Action += `. Voice-over: "人类的终结，没有爆炸。"`
	if prompt := Compose(clip); strings.Count(prompt, "人类的终结，没有爆炸。") != 1 {
		t.Fatalf("line repeated:\n%s", prompt)
	}
}

func TestMusicDialogueAndSilence(t *testing.T) {
	clip := narrated("seedance-2.0")
	clip.Music = "slow sparse piano with low strings."
	if prompt := Compose(clip); !strings.Contains(prompt, "(slow sparse piano with low strings plays quietly in the background under the voice") || strings.Contains(prompt, "No BGM") {
		t.Errorf("music:\n%s", prompt)
	}
	clip.Narration, clip.Lines = false, []Line{{Speaker: "Chen Wei", Text: "こんにちは", Language: "Japanese"}}
	clip.Cast[0].Voice = "tired, plain"
	if prompt := Compose(clip); !strings.Contains(prompt, "Chen Wei says in Japanese: {こんにちは}") || !strings.Contains(prompt, "their lips match the words") {
		t.Errorf("dialogue:\n%s", prompt)
	}
	clip.Audio = Silent
	if prompt := Compose(clip); !strings.Contains(prompt, "No audio.") || strings.Contains(prompt, "こんにちは") {
		t.Errorf("silent:\n%s", prompt)
	}
}

func TestALinePinnedToAShotIsWrittenUnderThatShot(t *testing.T) {
	clip := narrated("seedance-2.0")
	clip.Lines[1].Shot = 1
	prompt := Compose(clip)
	first, second, shotTwo := strings.Index(prompt, "人类的终结"), strings.Index(prompt, "我们把选择"), strings.Index(prompt, "Shot 2:")
	if !(second < shotTwo && shotTwo < first) || strings.Count(prompt, "我们把选择交给机器。") != 1 {
		t.Fatalf("pinned line is not under shot 1:\n%s", prompt)
	}
}

func TestTheWriterReadsOnlyTheSectionsItsClipNeeds(t *testing.T) {
	narration := narrated("seedance-2.0")
	if got := strings.Join(Sections(narration), " "); got != "index bindings first-frame shots-2.0 action camera speech-2.0 music-2.0 constraints example-2.0" {
		t.Errorf("narrated 2.0 clip reads: %s", got)
	}
	silent := narrated("seedance-2.5-flash")
	silent.Audio = Silent
	if got := strings.Join(Sections(silent), " "); got != "index bindings first-frame shots-2.5 action camera constraints example-2.5" {
		t.Errorf("silent 2.5 clip reads: %s", got)
	}
	system, facts := Brief(narration)
	for _, want := range []string{"Do NOT put second counts or timestamps on shots", "Put each spoken line in curly braces", "Specify only ONE type of camera movement", "Externalise emotion"} {
		if !strings.Contains(system, want) {
			t.Errorf("the 2.0 writer is not given %q", want)
		}
	}
	if strings.Contains(system, "1-second intervals") || strings.Contains(system, "Seedance 2.5 series)") {
		t.Error("the 2.0 writer is given 2.5 guidance")
	}
	if !strings.Contains(facts, `"spoken_lines_to_place_in_the_shot_they_fit"`) || !strings.Contains(facts, "人类的终结，没有爆炸。") || !strings.Contains(facts, `"draft"`) {
		t.Errorf("facts:\n%s", facts)
	}
	if len(system) > 12000 {
		t.Errorf("the writer's playbook is %d characters", len(system))
	}
}

func TestAWrittenPromptMustKeepTheFacts(t *testing.T) {
	clip := narrated("seedance-2.0")
	good := "A quiet end. Shot 1: wide shot, slow push-in; robots glide past. Voiceover: a low warm male voice says {人类的终结，没有爆炸。} Shot 2: Chen Wei turns his head and the voice continues {我们把选择交给机器。} No BGM. Keep it subtitle-free."
	if err := Check(clip, good); err != nil {
		t.Fatalf("a faithful prompt was refused: %v", err)
	}
	for name, bad := range map[string]string{
		"a line said twice":       good + " {人类的终结，没有爆炸。}",
		"a line reworded":         strings.Replace(good, "没有爆炸", "没有战争", 1),
		"a shot dropped":          strings.Replace(good, "Shot 2:", "Then", 1),
		"a shot invented":         good + " Shot 3: fireworks.",
		"an id leaked":            good + " (char_46d21012)",
		"music slipped in":        strings.Replace(good, "No BGM.", "(soft piano)", 1),
		"subtitles not ruled out": strings.Replace(good, "Keep it subtitle-free.", "", 1),
	} {
		if Check(clip, bad) == nil {
			t.Errorf("accepted a prompt with %s", name)
		}
	}
	if err := Check(clip, Compose(clip)); err != nil {
		t.Errorf("the plain draft itself fails the check: %v", err)
	}
}

// Pictures sent as references are defined once, in the order sent, and the check refuses
// a prompt that leaves one out or names one that was not sent.
func TestReferencePicturesAreBoundAndChecked(t *testing.T) {
	c := Clip{Family: "seedance-2.0", Summary: "Chen Wei meets The Visitor on a rooftop", Audio: Silent,
		Cast:       []Character{{Name: "Chen Wei", Look: "a tired man in a green jacket"}},
		References: []Reference{{Kind: "character", Name: "Chen Wei", Sheet: true}, {Kind: "place", Name: "Dusk City Rooftop"}},
		Shots:      []Shot{{Seconds: 5, Framing: "wide shot", Move: "slow push-in", Action: "Chen Wei stands at the railing"}}}
	draft := Compose(c)
	for _, want := range []string{"Define the person in @Image 1 as Chen Wei", "there is only one Chen Wei", "@Image 2 is the place Dusk City Rooftop"} {
		if !strings.Contains(draft, want) {
			t.Fatalf("draft lacks %q:\n%s", want, draft)
		}
	}
	if err := Check(c, draft); err != nil {
		t.Fatalf("draft refused: %v", err)
	}
	if err := Check(c, strings.Replace(draft, "@Image 2", "the rooftop", 1)); err == nil || !strings.Contains(err.Error(), "@Image 2 is never named") {
		t.Fatalf("missing picture: %v", err)
	}
	if err := Check(c, draft+" The light of @Image 3."); err == nil || !strings.Contains(err.Error(), "past the 2") {
		t.Fatalf("extra picture: %v", err)
	}
	if got := Sections(c); got[2] != "references" {
		t.Fatalf("sections = %v", got)
	}
	system, facts := Brief(c)
	if !strings.Contains(system, "## Reference pictures") || strings.Contains(system, "## The first frame") || !strings.Contains(facts, `"tag": "@Image 1"`) || strings.Contains(facts, "first_frame") {
		t.Fatalf("brief does not describe the pictures:\n%s", facts)
	}
	c.References = nil
	if got := Sections(c); got[2] != "first-frame" {
		t.Fatalf("sections without pictures = %v", got)
	}
}

// A voice recording sent with the clip is bound to its character once, and the check
// refuses a prompt that leaves it out or names one that was not sent.
func TestVoiceRecordingsAreBoundAndChecked(t *testing.T) {
	c := Clip{Family: "seedance-2.0", Summary: "Chen Wei on a rooftop", Audio: Native,
		References: []Reference{{Kind: "character", Name: "Chen Wei"}}, Voices: []string{"Chen Wei"},
		Shots: []Shot{{Seconds: 5, Action: "Chen Wei stands at the railing"}}, Lines: []Line{{Speaker: "Chen Wei", Text: "The city never sleeps", Shot: 1}}}
	draft := Compose(c)
	if !strings.Contains(draft, "Chen Wei speaks with the voice in @Audio 1") {
		t.Fatalf("draft does not bind the voice:\n%s", draft)
	}
	if err := Check(c, draft); err != nil {
		t.Fatalf("draft refused: %v", err)
	}
	if err := Check(c, strings.Replace(draft, "@Audio 1", "the recording", 1)); err == nil || !strings.Contains(err.Error(), "@Audio 1 is never named") {
		t.Fatalf("missing voice: %v", err)
	}
	if err := Check(c, draft+" Like @Audio 2."); err == nil || !strings.Contains(err.Error(), "past the 1") {
		t.Fatalf("extra voice: %v", err)
	}
	if _, facts := Brief(c); !strings.Contains(facts, "@Audio 1") {
		t.Fatalf("the writer is not told about the voice recording:\n%s", facts)
	}
}

// Gemini Omni is told, per shot, when each line starts, who says it in which language,
// and the exact words in quotes; Seedance's notation is left as it was.
func TestOmniLinesAreTimedAndQuoted(t *testing.T) {
	clip := Clip{
		Family: "gemini-omni-flash", Summary: "Mei asks Jun about money", Language: "Chinese", Audio: Native,
		Cast:  []Character{{Name: "Mei", Look: "Young woman in a grey coat", Voice: "warm, low"}, {Name: "Jun", Look: "Older man with glasses"}},
		Shots: []Shot{{Seconds: 3, Action: "Mei looks up"}, {Seconds: 2, Action: "Jun smiles"}, {Seconds: 3, Action: "They walk"}},
		Lines: []Line{
			{Speaker: "Mei", Text: "如果钱不需要靠银行来信任呢？", Language: "Chinese", Shot: 1, At: 0, Until: 3.1},
			{Speaker: "Jun", Text: "那就用代码。", Language: "Chinese", Shot: 2, At: 3, Until: 4.2},
		},
	}
	prompt := Compose(clip)
	for _, want := range []string{
		"Shot 1 (0-3s): Mei looks up.\nAt 0s, Mei says in Chinese (warm, low): \"如果钱不需要靠银行来信任呢？\"",
		"Shot 2 (3-5s): Jun smiles.\nAt 3s, Jun says in Chinese: \"那就用代码。\"",
		"Only one person speaks at a time",
		"word for word in Chinese",
		"Sound design:",
		"No subtitles",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Omni prompt lacks %q:\n%s", want, prompt)
		}
	}
	if err := Check(clip, prompt); err != nil {
		t.Fatalf("Omni prompt refused: %v\n%s", err, prompt)
	}
	sections := strings.Join(Sections(clip), ",")
	if !strings.Contains(sections, "speech-omni") || strings.Contains(sections, "speech-2.") {
		t.Fatalf("Omni playbook: %s", sections)
	}
	_, facts := Brief(clip)
	if !strings.Contains(facts, `"at": "0-3.1s"`) {
		t.Fatalf("the writer is not told when a line is said:\n%s", facts)
	}
	// Seedance keeps its own notation and is never given times.
	clip.Family = "seedance-2.5"
	if prompt := Compose(clip); !strings.Contains(prompt, `Mei's line (warm, low): "如果钱不需要靠银行来信任呢？"`) || strings.Contains(prompt, "At 0s") {
		t.Fatalf("Seedance notation changed:\n%s", prompt)
	}
	if _, facts := Brief(clip); strings.Contains(facts, `"at"`) {
		t.Fatalf("Seedance writer was given times:\n%s", facts)
	}
}

// A clip of a story made in several clips has voices and the scene's ambience, and no
// music: one bed is laid under the whole film in the edit.
func TestClipOfAMultiClipStoryAsksForAmbienceAndNoMusic(t *testing.T) {
	base := Clip{Summary: "Two friends wait out the rain", Audio: Native, MusicInEdit: true, Ambience: "steady rain on a tin roof, distant traffic.",
		Shots: []Shot{{Seconds: 4, Framing: "wide shot", Move: "fixed camera", Action: "Mia looks out of the window"}},
		Lines: []Line{{Speaker: "Mia", Text: "It will stop soon.", Language: "English", Shot: 1, At: 1, Until: 2.5}}}
	for family, want := range map[string][]string{
		"gemini-omni-1.2-flash": {"Sound design: steady rain on a tin roof, distant traffic, quiet under the voices.", "No music: do not add background music"},
		"seedance-2.5-flash":    {"Ambient sound: steady rain on a tin roof, distant traffic, steady through every shot", "No music of any kind"},
		"seedance-2.0":          {"< steady rain on a tin roof, distant traffic can be heard throughout, under the voice >", "No BGM; generate only voices, environmental sounds and action sounds."},
	} {
		c := base
		c.Family = family
		prompt := Compose(c)
		for _, w := range want {
			if !strings.Contains(prompt, w) {
				t.Errorf("%s: missing %q in\n%s", family, w, prompt)
			}
		}
		if err := Check(c, prompt); err != nil {
			t.Errorf("%s: %v", family, err)
		}
		_, facts := Brief(c)
		if !strings.Contains(facts, "one music track is laid under the whole film") || !strings.Contains(facts, "steady rain on a tin roof") {
			t.Errorf("%s: the writer is not told the sound plan:\n%s", family, facts)
		}
		// Nobody speaking keeps the ambience too.
		c.Lines = nil
		if prompt = Compose(c); !strings.Contains(prompt, "steady rain on a tin roof") || Check(c, prompt) != nil {
			t.Errorf("%s: a clip without speech lost its ambience or its no-music line:\n%s", family, prompt)
		}
	}
}

// A single clip may still make its own music, as before.
func TestSingleClipKeepsItsOwnMusic(t *testing.T) {
	c := Clip{Family: "gemini-omni-1.2-flash", Summary: "A quiet end", Audio: Native, Music: "slow sparse piano",
		Shots: []Shot{{Seconds: 6, Action: "Robots glide past"}}}
	prompt := Compose(c)
	if !strings.Contains(prompt, "Background music: slow sparse piano") || strings.Contains(prompt, "No music") || Check(c, prompt) != nil {
		t.Fatal(prompt)
	}
}
