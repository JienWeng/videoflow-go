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
