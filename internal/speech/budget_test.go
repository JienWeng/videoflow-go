package speech

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.02 }

func TestCountCharactersAndWords(t *testing.T) {
	for _, c := range []struct {
		text, language string
		chars, words   int
	}{
		{"如果钱不需要靠银行来信任呢？", "Chinese", 13, 0},
		{"我们用AI来做", "zh", 5, 1},
		{"2026年", "Mandarin Chinese", 5, 0},
		{"Don't wait for the bank.", "English", 0, 5},
		{"¿Y si el dinero no necesitara bancos?", "Spanish", 0, 7},
		{"2026 is here", "en", 0, 3},
		{"", "en", 0, 0},
	} {
		chars, words := Count(c.text, c.language)
		if chars != c.chars || words != c.words {
			t.Errorf("Count(%q) = %d chars %d words, want %d and %d", c.text, chars, words, c.chars, c.words)
		}
	}
}

func TestCodeNames(t *testing.T) {
	for in, want := range map[string]string{"Chinese": "zh", "Mandarin Chinese": "zh", "zh-CN": "zh", "中文": "zh", "普通话": "zh", "English": "en", "en-US": "en", "Español": "es", "spanish": "es", "Klingon": "", "": ""} {
		if got := Code(in); got != want {
			t.Errorf("Code(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecondsByLanguage(t *testing.T) {
	// 13 characters at 4.2 a second; the question mark ends the line, so no pause.
	if got := Seconds("如果钱不需要靠银行来信任呢？", "Chinese", nil); !near(got, 13/4.2) {
		t.Errorf("Chinese line = %.2f s", got)
	}
	// Two clauses: one pause inside the line.
	if got := Seconds("钱是什么，谁来信任它？", "zh", nil); !near(got, 9/4.2+Pause) {
		t.Errorf("Chinese clauses = %.2f s", got)
	}
	if got := Seconds("What if money did not need a bank?", "English", nil); !near(got, 8/2.5) {
		t.Errorf("English line = %.2f s", got)
	}
	if got := Seconds("¿Y si el dinero no necesitara bancos?", "Spanish", nil); !near(got, 7/2.6) {
		t.Errorf("Spanish line = %.2f s", got)
	}
	// An unknown language blends: characters at 4.2, words at 2.4.
	if got := Seconds("Hola 你好", "Klingon", nil); !near(got, 2/4.2+1/2.4) {
		t.Errorf("blend = %.2f s", got)
	}
	// A measured rate replaces the built-in one.
	if got := Seconds("你好你好", "zh", Calibration{"zh": {Chars: 2}}); !near(got, 2) {
		t.Errorf("calibrated = %.2f s", got)
	}
}

func TestFitVerdicts(t *testing.T) {
	// The real project's 8-second scene had one 14-character line: far too little.
	f := FitLines([]Line{{ID: "a", Text: "如果钱不需要靠银行来信任呢？", Language: "zh", Hint: -1}}, 8, nil)
	if f.Verdict != OK || f.Free < 4.5 {
		t.Fatalf("short line fit = %+v", f)
	}
	// About 30 characters in two lines fills 8 seconds without running over.
	lines := []Line{
		{ID: "a", Text: "如果钱不需要靠银行来信任呢？", Language: "zh", Hint: -1},
		{ID: "b", Text: "那我们就用代码来建立信任。", Language: "zh", Hint: -1},
	}
	f = FitLines(lines, 8, nil)
	if f.Verdict != Tight && f.Verdict != OK {
		t.Fatalf("two Chinese lines in 8 s = %+v", f)
	}
	if f.Lines[1].Start <= f.Lines[0].End {
		t.Fatalf("the second line must start after the first and a breath: %+v", f.Lines)
	}
	// The same lines in a 3-second shot cannot be said.
	f = FitLines(lines, 3, nil)
	if f.Verdict != TooLong || f.Lines[0].Verdict != TooLong || f.Lines[1].Verdict != TooLong {
		t.Fatalf("3-second fit = %+v", f)
	}
	if f.Free != 0 {
		t.Fatalf("free seconds never go below zero: %+v", f)
	}
	// English: 18 words in 8 s is 7.2 s, tight; 25 words is too long.
	f = FitLines([]Line{{ID: "e", Text: "What if money did not need a bank to be trusted, and every one of us could check it?", Language: "English", Hint: -1}}, 8, nil)
	if f.Verdict != Tight {
		t.Fatalf("English fit = %+v", f)
	}
	f = FitLines([]Line{{ID: "s", Text: "¿Y si el dinero no necesitara un banco para ser de confianza, y cada uno de nosotros pudiera comprobarlo cuando quisiera, sin pedir permiso?", Language: "Spanish", Hint: -1}}, 8, nil)
	if f.Verdict != TooLong {
		t.Fatalf("Spanish fit = %+v", f)
	}
	// A hint starts a line later than it would follow on.
	f = FitLines([]Line{{ID: "h", Text: "你好", Language: "zh", Hint: 2}}, 3, nil)
	if !near(f.Lines[0].Start, 2) || f.Verdict != OK {
		t.Fatalf("hinted fit = %+v", f)
	}
}

func TestBudget(t *testing.T) {
	b := BudgetFor(8, "Chinese", nil)
	if b.Unit != "characters" || b.Low != 23 || b.High != 30 || b.Max != 33 {
		t.Fatalf("Chinese 8 s budget = %+v", b)
	}
	b = BudgetFor(8, "English", nil)
	if b.Unit != "words" || b.Low != 14 || b.High != 18 || b.Max != 20 {
		t.Fatalf("English 8 s budget = %+v", b)
	}
	b = BudgetFor(3, "es", nil)
	if b.Unit != "words" || b.High != 7 {
		t.Fatalf("Spanish 3 s budget = %+v", b)
	}
}

func TestMusicRoomIsAnEighthOfTheClipWithinBounds(t *testing.T) {
	for seconds, want := range map[float64]float64{3: 0.8, 8: 1, 10: 1.25, 15: 1.88, 30: 2} {
		if got := MusicRoom(seconds); got != want {
			t.Errorf("MusicRoom(%v) = %v, want %v", seconds, got, want)
		}
	}
	// The room sits inside what a script aims to leave free.
	for _, seconds := range []float64{4, 8, 12, 15} {
		if MusicRoom(seconds) > seconds*(1-Target[1])+0.5 {
			t.Errorf("the room for music in %vs is more than a script leaves", seconds)
		}
	}
}
