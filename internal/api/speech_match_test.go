package api

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSpeechSimilarityCountsCharactersForChineseAndWordsOtherwise(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"他从一万年后回来，", "他从一万年后回来", 1},
		{"说话", "說話", 1},                                          // traditional and simplified are the same words
		{"我们走吧！", "我们走", 0.75},                                   // 3 of 4 characters
		{"守住机器", "守住机械", 0.75},                                   // one character of four differs
		{"It won't fly. I give up.", "it wont fly i give up", 1}, // punctuation, case, apostrophes
		{"Wait. I know a secret.", "Wait I now a secret", 0.8},   // one word of five
		{"Hello there", "", 0},
		{"", "", 1},
		{"ＡＢＣ１２３", "abc123", 1}, // full-width letters and digits
	}
	for _, c := range cases {
		if got := speechSimilarity(c.a, c.b); got != c.want {
			t.Errorf("similarity(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if units := speechUnits("我叫Mia，你好 world"); strings.Join(units, "|") != "我|叫|mia|你|好|world" {
		t.Fatalf("units: %q", units)
	}
}

func TestSimplifiedTableIsPairs(t *testing.T) {
	for trad, simp := range simplifiedOf {
		if utf8.RuneLen(trad) != 3 || utf8.RuneLen(simp) != 3 || trad == simp {
			t.Fatalf("bad pair %q→%q", trad, simp)
		}
	}
	if len(simplifiedOf) < 300 {
		t.Fatalf("only %d characters fold", len(simplifiedOf))
	}
}

func TestSpeechScriptRecognizesTheWritingSystem(t *testing.T) {
	for text, want := range map[string]string{"他从一万年后回来": scriptHan, "こんにちは、元気ですか": scriptKana, "안녕하세요 친구": scriptHangul, "Where did the towers come from": scriptLatin, "ok": ""} {
		if got := speechScript(text); got != want {
			t.Errorf("speechScript(%q) = %q, want %q", text, got, want)
		}
	}
	if !sameScript(scriptKana, scriptHan) || sameScript(scriptHan, scriptLatin) || !sameScript(scriptHan, "") {
		t.Fatal("sameScript")
	}
}

// A paraphrased line sits where it was said between two lines that were heard as written.
func TestAlignHeardGivesAReworkedLineTheSpeechInItsPlace(t *testing.T) {
	words := spoken("他从", 0.6, 1.0, "一万年", 1.0, 1.6, "后", 1.6, 1.8, "回来", 1.8, 2.4,
		"我们", 3.0, 3.3, "快", 3.3, 3.5, "跑", 3.5, 3.8,
		"守住", 5.5, 6.0, "机器", 6.0, 6.6)
	matches, extra := alignHeard([]string{"他从一万年后回来，", "大家赶紧离开这里！", "守住机器——"}, words)
	if !matches[0].heard() || matches[0].Guessed || matches[0].First != 0 || matches[0].Last != 3 {
		t.Fatalf("line 1: %+v", matches[0])
	}
	if !matches[1].Guessed || matches[1].First != 4 || matches[1].Last != 6 {
		t.Fatalf("line 2 should take the speech between its neighbours: %+v", matches[1])
	}
	if !matches[2].heard() || matches[2].First != 7 || len(extra) != 0 {
		t.Fatalf("line 3: %+v extra %v", matches[2], extra)
	}
	// heardSpans keeps reporting only lines heard as written.
	if spans := heardSpans([]string{"他从一万年后回来，", "大家赶紧离开这里！", "守住机器——"}, words); spans[1][0] >= 0 || spans[2] != [2]float64{5.5, 6.6} {
		t.Fatalf("spans %v", spans)
	}
}

func chineseLines() []timedLine {
	return []timedLine{
		{ID: "l1", Text: "他从一万年后回来，", Language: "Chinese", Start: 0.6, End: 3, Shot: "s1", Timing: timingEstimated},
		{ID: "l2", Text: "守住这台机器，不要让任何人靠近。", Language: "Chinese", Start: 4.1, End: 7.4, Shot: "s2", Timing: timingEstimated},
	}
}

// Captions keep the script's wording when it was said, and show what was said (with the
// script kept) when the video model said something else.
func TestPlaceHeardLinesKeepsScriptWordingOnlyWhenItWasSaid(t *testing.T) {
	words := spoken("他从", 0.6, 1.0, "一万年", 1.0, 1.6, "后", 1.6, 1.8, "回来", 1.8, 2.4,
		"我们", 4.2, 4.6, "快点", 4.6, 5.0, "走吧", 5.0, 5.4)
	got := placeHeardLines(chineseLines(), words, 8)
	if got[0].Text != "他从一万年后回来，" || got[0].Scripted != "" || got[0].Timing != timingHeard || got[0].Start != 0.52 || got[0].End != 2.7 {
		t.Fatalf("line 1: %+v", got[0])
	}
	if got[1].Text != "我们快点走吧" || got[1].Scripted != "守住这台机器，不要让任何人靠近。" || got[1].Timing != timingHeard || got[1].Start != 4.12 || got[1].End != 5.7 {
		t.Fatalf("line 2: %+v", got[1])
	}
	none := placeHeardLines(chineseLines(), nil, 8)
	if none[0].Timing != timingEstimated || none[0].Start != 0.6 {
		t.Fatalf("%+v", none[0])
	}
}
