package seedance

import (
	"strings"
	"testing"
)

func TestSpokenTextReadsChineseYearsDigitByDigit(t *testing.T) {
	for in, want := range map[string]string{
		"1977年，他出生了。":   "一九七七年，他出生了。",
		"在2024 年":       "在二零二四年",
		"1970年代的音乐":     "一九七零年代的音乐",
		"他有1977个比特币。":   "他有1977个比特币。",
		"Born in 1977.": "Born in 1977.",
	} {
		if got := SpokenText(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

// Spans of time are numbers, not years: 过了50年 is 五十年, never 五零年.
func TestSpokenTextLeavesDurationsAndOtherNumbersAlone(t *testing.T) {
	for in, want := range map[string]string{
		"过了50年，他回来了。": "过了50年，他回来了。",
		"20年后":        "20年后",
		"10年前我们见过。":   "10年前我们见过。",
		"这棵树活了100年。":  "这棵树活了100年。",
		"编号12345年":    "编号12345年",
		"70年代的歌":      "七零年代的歌",
		"１９７７年，他出生了。": "一九七七年，他出生了。",
		"1977年到1980年": "一九七七年到一九八零年",
	} {
		if got := SpokenText(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestOmniPromptSaysChineseYearsDigitByDigit(t *testing.T) {
	c := Clip{Family: "gemini-omni", Audio: Native, Lines: []Line{{Speaker: "Mei", Text: "1977年，我们开始了。", Language: "Chinese"}}, Shots: []Shot{{Seconds: 4, Action: "Mei talks"}}}
	_, facts := Brief(c)
	if !strings.Contains(facts, "一九七七年") || strings.Contains(facts, "1977") {
		t.Fatal(facts)
	}
	if err := Check(c, `Shot 1: Mei says in Chinese: "一九七七年，我们开始了。" Sound design: quiet room. No music.`); err != nil && strings.Contains(err.Error(), "spoken line") {
		t.Fatal(err)
	}
}
