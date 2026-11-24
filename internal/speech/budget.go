// Package speech estimates how long spoken lines take to say, so a script can be sized
// to the clips it is spoken in before any video is paid for. The estimate is
// deterministic: characters (Chinese, Japanese, Korean) or words (languages written with
// spaces) divided by a calm speaking rate for the language, plus a short pause at each
// clause break inside a line and between lines.
//
// How the rates were picked. They are for calm, acted dialogue, a little slower than read
// speech, so an estimate errs on the long side and a line that "fits" leaves room.
//   - Mandarin: each Chinese character is one syllable. Pellegrino, Coupé & Marsico
//     (2011, "A cross-language perspective on speech information rate", Language 87(3))
//     measured read Mandarin at about 5.2 syllables a second; broadcast Mandarin is
//     commonly quoted at 4–5 characters a second. Dialogue with feeling is slower: 4.2.
//   - Japanese: the same study measured about 7.8 morae a second; written Japanese mixes
//     kanji (often two morae) and kana (one), which comes to roughly 5 characters a second
//     read aloud. Acted: 4.5.
//   - Korean: one Hangul block is one syllable; read Korean runs at about 5–6 syllables a
//     second. Acted: 4.5.
//   - English: conversational English is about 150 words a minute (2.5 a second); the
//     study's syllable rate (about 6.2/s) at ~1.4 syllables a word gives ~4 words a second
//     read quickly, which is too fast to act. 2.5.
//   - Spanish, French, Italian, Portuguese: more syllables a word but spoken faster
//     (Spanish ≈7.8, French ≈7.2 syllables a second in the study), about the same words a
//     second as English. 2.6. German: longer compound words, ≈6.0 syllables a second. 2.3.
//   - Anything else: 2.4 words a second; characters from the CJK scripts inside any text
//     are counted at 4.2 a second, so mixed text ("我们用AI来做") is a blend.
//   - Pause: about a third of a second at each comma, full stop or other clause break
//     inside a line, and between consecutive lines (a breath, or one speaker handing over
//     to the next).
//
// These figures come from the literature cited and were not measured on the video models
// themselves; Google documents no speaking rate for Gemini Omni. Calibration (measured
// rates from real recordings, for example the saved voice samples) replaces a language's
// rate when it is known.
package speech

import (
	"math"
	"strings"
	"unicode"
)

// Pause is the time added at a clause break inside a line and between two lines.
const Pause = 0.35

// Verdicts.
const (
	OK      = "ok"       // fits with room to spare (at most 90 % of the time)
	Tight   = "tight"    // fits, but uses more than 90 % of the time
	TooLong = "too_long" // cannot be said in the time
)

// TightShare is the share of the time above which speech is tight.
const TightShare = 0.9

// Target is the share of a shot's seconds a script writer aims to fill: low and high.
var Target = [2]float64{0.7, 0.9}

// Rate is how fast a language is spoken: CJK characters a second and words a second.
type Rate struct {
	Chars float64 `json:"chars_per_second"`
	Words float64 `json:"words_per_second"`
}

var rates = map[string]Rate{
	"zh": {Chars: 4.2, Words: 2.4},
	"ja": {Chars: 4.5, Words: 2.4},
	"ko": {Chars: 4.5, Words: 2.4},
	"en": {Chars: 4.2, Words: 2.5},
	"es": {Chars: 4.2, Words: 2.6},
	"fr": {Chars: 4.2, Words: 2.6},
	"it": {Chars: 4.2, Words: 2.6},
	"pt": {Chars: 4.2, Words: 2.6},
	"de": {Chars: 4.2, Words: 2.3},
}

var fallback = Rate{Chars: 4.2, Words: 2.4}

// Calibration holds measured rates by language code; they replace the built-in ones.
type Calibration map[string]Rate

// RateFor is the speaking rate of a language, measured when calibrated.
func RateFor(language string, cal Calibration) Rate {
	code := Code(language)
	r, ok := rates[code]
	if !ok {
		r = fallback
	}
	if m, ok := cal[code]; ok {
		if m.Chars > 0 {
			r.Chars = m.Chars
		}
		if m.Words > 0 {
			r.Words = m.Words
		}
	}
	return r
}

// Code is a language's two-letter code, from a code or an English or native name; empty
// when unknown.
func Code(language string) string {
	l := strings.ToLower(strings.TrimSpace(language))
	if l == "" {
		return ""
	}
	for _, c := range codes {
		for _, name := range c.names {
			if l == name || strings.HasPrefix(l, name+" ") || strings.HasPrefix(l, name+"-") || strings.HasPrefix(l, name+"_") || strings.HasPrefix(l, name+"(") {
				return c.code
			}
		}
	}
	return ""
}

var codes = []struct {
	code  string
	names []string
}{
	{"zh", []string{"zh", "cmn", "yue", "chinese", "mandarin", "cantonese", "中文", "汉语", "漢語", "普通话", "國語", "国语", "粤语", "華語", "华语"}},
	{"ja", []string{"ja", "japanese", "日本語"}},
	{"ko", []string{"ko", "korean", "한국어"}},
	{"en", []string{"en", "english"}},
	{"es", []string{"es", "spanish", "español", "espanol", "castellano"}},
	{"fr", []string{"fr", "french", "français", "francais"}},
	{"de", []string{"de", "german", "deutsch"}},
	{"it", []string{"it", "italian", "italiano"}},
	{"pt", []string{"pt", "portuguese", "português", "portugues"}},
}

// CharBased says whether a language's length is counted in characters, not words.
func CharBased(language string) bool {
	switch Code(language) {
	case "zh", "ja", "ko":
		return true
	}
	return false
}

// cjk reports a character of a script written without spaces between words, counted
// one by one: Chinese characters, Japanese kana and Korean Hangul.
func cjk(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) || r == 'ー'
}

// clauseBreak is punctuation where a speaker pauses.
func clauseBreak(r rune) bool {
	return strings.ContainsRune(",.;:!?，。；：！？、…—", r)
}

// Count is the length of a text: its CJK characters and its other words. In a
// character-based language a run of digits is read digit by digit ("2026" is four
// syllables in Chinese), so its digits count as characters.
func Count(text, language string) (chars, words int) {
	charDigits := CharBased(language)
	inWord := false
	for _, r := range text {
		switch {
		case cjk(r):
			chars++
			inWord = false
		case charDigits && unicode.IsDigit(r):
			chars++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				words++
			}
			inWord = true
		case (r == '\'' || r == '’') && inWord:
			// "don't" stays one word
		default:
			inWord = false
		}
	}
	return chars, words
}

// pauses counts the clause breaks inside a text: a run of break marks counts once, and
// the marks that end the text do not count.
func pauses(text string) int {
	text = strings.TrimRightFunc(strings.TrimSpace(text), func(r rune) bool {
		return clauseBreak(r) || unicode.IsSpace(r) || strings.ContainsRune("\"'”’」』)）", r)
	})
	n, prev := 0, false
	for _, r := range text {
		b := clauseBreak(r)
		if b && !prev {
			n++
		}
		prev = b
	}
	return n
}

// Seconds is how long a line takes to say at a calm pace.
func Seconds(text, language string, cal Calibration) float64 {
	chars, words := Count(text, language)
	if chars == 0 && words == 0 {
		return 0
	}
	r := RateFor(language, cal)
	return float64(chars)/r.Chars + float64(words)/r.Words + float64(pauses(text))*Pause
}

// Line is one line to fit. Hint is when it should start, in seconds from the start of
// the time it is fitted in; negative when it simply follows the line before.
type Line struct {
	ID       string
	Text     string
	Language string
	Hint     float64
}

// LineFit is one line's estimate: how long it takes, when it starts and ends, and
// whether it is still being said when the time is up.
type LineFit struct {
	ID      string  `json:"id"`
	Seconds float64 `json:"seconds"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Verdict string  `json:"verdict"`
}

// Fit is how a set of lines fits in some seconds.
type Fit struct {
	Seconds float64   `json:"seconds"`
	Used    float64   `json:"used"`
	Free    float64   `json:"free"`
	Verdict string    `json:"verdict"`
	Lines   []LineFit `json:"lines"`
}

// Verdict judges used seconds against the time available.
func Verdict(used, seconds float64) string {
	switch {
	case used > seconds+1e-9:
		return TooLong
	case used > seconds*TightShare+1e-9:
		return Tight
	}
	return OK
}

// FitLines says how lines spoken one after another fit in seconds. A line starts after
// the line before it and a breath, or at its hint when that is later.
func FitLines(lines []Line, seconds float64, cal Calibration) Fit {
	f := Fit{Seconds: seconds, Lines: []LineFit{}}
	cursor := 0.0
	for i, l := range lines {
		d := Seconds(l.Text, l.Language, cal)
		start := cursor
		if i > 0 && d > 0 {
			start += Pause
		}
		if l.Hint >= 0 && l.Hint > start {
			start = l.Hint
		}
		if d == 0 {
			start = cursor
		}
		end := start + d
		f.Lines = append(f.Lines, LineFit{ID: l.ID, Seconds: Round(d), Start: Round(start), End: Round(end), Verdict: Verdict(end, seconds)})
		cursor = end
	}
	f.Used = Round(cursor)
	f.Free = Round(math.Max(0, seconds-cursor))
	f.Verdict = Verdict(cursor, seconds)
	return f
}

// MusicRoom is how much of a clip is kept free of speech, so its ambience and the film's
// music bed are heard on their own for a moment: a short intro before the first line, a
// pause, or a breath at the end. About an eighth of the clip, at least 0.8 s and at
// most 2 s. It sits inside the 10-30 % a script leaves free (Target), so a clip that
// fits its speech budget usually has it; it is reported, never enforced.
func MusicRoom(seconds float64) float64 {
	return Round(math.Min(2, math.Max(0.8, seconds/8)))
}

// Round keeps two decimals, enough for display and comparison.
func Round(v float64) float64 { return math.Round(v*100) / 100 }

// Budget is how much may be said in some seconds, in the unit the language is counted
// in: the low and high end of the writer's target (70–90 % of the time), and the most
// that fits at all.
type Budget struct {
	Unit string `json:"unit"` // "characters" or "words"
	Low  int    `json:"low"`
	High int    `json:"high"`
	Max  int    `json:"max"`
}

// BudgetFor is how many characters or words fill seconds in a language.
func BudgetFor(seconds float64, language string, cal Calibration) Budget {
	return BudgetShare(seconds, language, cal, Target, 1)
}

// RepairTarget is the share of a shot's seconds a line rewritten after the speech check
// heard it cut off, changed or missing aims to fill: shorter than Target, with room
// for the video model's own pace. RepairMost is the most it may fill.
var RepairTarget = [2]float64{0.6, 0.75}

const RepairMost = 0.75

// BudgetShare is how many characters or words fill the shares target (low, high) and
// most of seconds in a language.
func BudgetShare(seconds float64, language string, cal Calibration, target [2]float64, most float64) Budget {
	r := RateFor(language, cal)
	rate, unit := r.Words, "words"
	if CharBased(language) {
		rate, unit = r.Chars, "characters"
	}
	return Budget{Unit: unit, Low: int(math.Floor(seconds * target[0] * rate)), High: int(math.Floor(seconds * target[1] * rate)), Max: int(math.Floor(seconds * most * rate))}
}
