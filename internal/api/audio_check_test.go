package api

import (
	"strings"
	"testing"
)

func issueCodes(r selfCheckResult) string {
	codes := []string{}
	for _, i := range r.Issues {
		codes = append(codes, i.Code)
	}
	return strings.Join(codes, ",")
}

func TestSpeechCheckPassesLinesSaidAsWritten(t *testing.T) {
	words := spoken("他從", 0.6, 1.0, "一萬年", 1.0, 1.6, "後", 1.6, 1.8, "回來", 1.8, 2.4,
		"守住", 4.2, 4.6, "这台", 4.6, 5.0, "机器", 5.0, 5.4, "不要让", 5.6, 6.1, "任何人", 6.1, 6.6, "靠近", 6.6, 7.0)
	r := judgeClipAudio(chineseLines(), words, 8, "Omni")
	if r.Verdict != verdictPass || len(r.Issues) != 0 || r.Score != 100 || r.Retry.WorthIt {
		t.Fatalf("%+v", r)
	}
	a := r.Audio
	if a.Lines[0].Status != audioLineOK || *a.Lines[0].Start != 0.6 || *a.Lines[0].End != 2.4 || *a.Lines[1].End != 7 || a.HeardLanguage != "Chinese" || a.ExpectedLanguage != "Chinese" || a.CutOff {
		t.Fatalf("%+v", a)
	}
}

func TestSpeechCheckFindsSpeechCutOffAtTheEnd(t *testing.T) {
	words := spoken("他从", 0.6, 1.0, "一万年", 1.0, 1.6, "后", 1.6, 1.8, "回来", 1.8, 2.4,
		"守住", 4.2, 4.6, "这台", 4.6, 5.0, "机器", 5.0, 5.4, "不要让", 5.6, 6.6, "任", 7.6, 7.9)
	r := judgeClipAudio(chineseLines(), words, 8, "Omni")
	if r.Verdict != verdictFix || !strings.HasPrefix(issueCodes(r), "speech_cut_off") || !r.Audio.CutOff || r.Audio.Lines[1].Status != audioLineCutOff {
		t.Fatalf("%s %+v", issueCodes(r), r.Audio.Lines)
	}
	if !strings.Contains(r.Issues[0].Detail, "Speech cut off at 7.9 s") || !strings.Contains(r.Retry.Change, "fewer words") || !r.Retry.WorthIt {
		t.Fatalf("%+v", r)
	}
	// The same speech, finished well before the end, is fine.
	finished := judgeClipAudio(chineseLines()[:1], words[:4], 8, "Omni")
	if finished.Verdict != verdictPass {
		t.Fatalf("%+v", finished)
	}
	// Finished, but the last word runs into the cut: a low note, not a fix.
	atEnd := judgeClipAudio(chineseLines()[:1], spoken("他从", 5.6, 6.0, "一万年", 6.0, 6.6, "后", 6.6, 7.0, "回来", 7.2, 7.85), 8, "Omni")
	if atEnd.Verdict != verdictPass || !strings.Contains(issueCodes(atEnd), "speech_at_end") || !strings.Contains(issueCodes(atEnd), "late_start") {
		t.Fatalf("%s", issueCodes(atEnd))
	}
}

func TestSpeechCheckFindsDifferentWordsMissingLinesAndExtras(t *testing.T) {
	words := spoken("他从", 0.6, 1.0, "一万年", 1.0, 1.6, "后", 1.6, 1.8, "回来", 1.8, 2.4,
		"我们", 4.2, 4.6, "快点", 4.6, 5.0, "走吧", 5.0, 5.4)
	r := judgeClipAudio(chineseLines(), words, 8, "Omni")
	if r.Verdict != verdictFix || issueCodes(r) != "different_words" || r.Issues[0].Severity != "high" || !strings.Contains(r.Issues[0].Detail, "Omni spoke different words for line 2") {
		t.Fatalf("%s %+v", issueCodes(r), r.Issues)
	}
	if r.Audio.Lines[1].Heard != "我们快点走吧" || r.Audio.Lines[1].Status != audioLineChanged || !strings.Contains(r.Retry.Change, "word for word") {
		t.Fatalf("%+v %q", r.Audio.Lines[1], r.Retry.Change)
	}
	// Only the first line was said: the second is missing (no speech in its place).
	missing := judgeClipAudio(chineseLines(), words[:4], 8, "Omni")
	if issueCodes(missing) != "line_missing" || missing.Score != 50 {
		t.Fatalf("%s %d", issueCodes(missing), missing.Score)
	}
	// Speech nobody wrote.
	extra := judgeClipAudio(chineseLines()[:1], spoken("他从一万年后回来", 0.6, 2.4, "哈哈哈哈好的没问题", 3.0, 4.0), 8, "Omni")
	if issueCodes(extra) != "extra_speech" || extra.Verdict != verdictFix || extra.Audio.Extra != "哈哈哈哈好的没问题" {
		t.Fatalf("%s %+v", issueCodes(extra), extra.Audio)
	}
}

func TestSpeechCheckFindsTheWrongLanguage(t *testing.T) {
	words := spoken("He", 0.5, 0.7, "came", 0.7, 1.0, "back", 1.0, 1.3, "from", 1.3, 1.5, "the", 1.5, 1.6, "future", 1.6, 2.2)
	r := judgeClipAudio(chineseLines(), words, 8, "Omni")
	if r.Verdict != verdictFail || issueCodes(r) != "wrong_language" || r.Score > 20 {
		t.Fatalf("%s %+v", issueCodes(r), r)
	}
	if !strings.Contains(r.Issues[0].Detail, "Omni spoke English or another Latin-alphabet language, not Chinese") || !strings.Contains(r.Retry.Change, "in Chinese only") {
		t.Fatalf("%q %q", r.Issues[0].Detail, r.Retry.Change)
	}
	japanese := judgeClipAudio(chineseLines()[:1], spoken("彼は", 0.5, 1.0, "未来から", 1.0, 1.8, "来ました", 1.8, 2.5), 8, "Omni")
	if issueCodes(japanese) != "wrong_language" || japanese.Audio.HeardLanguage != "Japanese" {
		t.Fatalf("%s %+v", issueCodes(japanese), japanese.Audio)
	}
}

func TestSpeechCheckWithoutSpeech(t *testing.T) {
	r := judgeClipAudio(chineseLines(), nil, 8, "Omni")
	if r.Verdict != verdictFail || issueCodes(r) != "no_speech" {
		t.Fatalf("%+v", r)
	}
	quiet := judgeClipAudio(nil, nil, 8, "Omni")
	if quiet.Verdict != verdictPass || len(quiet.Issues) != 0 {
		t.Fatalf("%+v", quiet)
	}
	chatter := judgeClipAudio(nil, spoken("Hello", 1.0, 1.4, "everyone", 1.4, 2.0), 8, "Omni")
	if issueCodes(chatter) != "unexpected_speech" {
		t.Fatalf("%+v", chatter)
	}
}
