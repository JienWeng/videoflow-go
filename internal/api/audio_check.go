package api

import (
	"context"
	"fmt"
	"math"
	"strings"

	"videoflow-go/internal/providers"
)

// Speech check: after a clip is made with its own sound, the clip is listened to with
// the Listening service chosen in AI setup (the same transcript caption timing uses,
// so a clip is paid for once: about half a cent per minute) and what was heard is
// compared with the lines the scene's script gives it: were they said, in the right
// language, with the right words, and did the clip end before a line was finished.
// It is the self-check kind clip_audio, stored as the clip's qa_json.audio_check, and
// like every self-check it never blocks: without a listening key it says how to add
// one and reads as "pass". A clip is never made again because of it.

const selfCheckClipAudio = "clip_audio"

// audioCheckVersion is part of the speech check's hash: a changed check runs again.
const audioCheckVersion = "audio-check-1"

// Thresholds of the speech check.
const (
	// audioCutMargin: speech whose last word ends this close to the clip's end runs
	// into the cut. With the line unfinished it was cut off.
	audioCutMargin = 0.25
	// audioWordsKept is the similarity from which a line counts as said as written;
	// below audioWordsLost the words are mostly different.
	audioWordsKept = heardWordingKept
	audioWordsLost = 0.4
	// audioLateStart: speech that starts after max(1.5 s, 35 % of the clip) is late.
	audioLateStartSeconds, audioLateStartShare = 1.5, 0.35
	// audioOffShot: a line heard more than max(2 s, 30 % of the clip) away from its
	// shot's estimated time is in the wrong shot.
	audioOffShotSeconds, audioOffShotShare = 2.0, 0.3
	// audioExtraShare: unscripted speech of at least this share of what was heard (and
	// four letters) is reported.
	audioExtraShare = 0.25
)

// Line statuses.
const (
	audioLineOK      = "ok"      // said as written
	audioLineChanged = "changed" // something else was said where it belongs
	audioLineMissing = "missing" // not heard
	audioLineCutOff  = "cut_off" // started but the clip ended before it was finished
)

// audioLine is one script line as heard.
type audioLine struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Heard is what was said in its place; Similarity how close that is (0-1):
	// character by character for Chinese and Japanese, word by word otherwise.
	Heard      string  `json:"heard"`
	Similarity float64 `json:"similarity"`
	// Start and End are when its words are spoken, in the clip's own seconds.
	Start *float64 `json:"start,omitempty"`
	End   *float64 `json:"end,omitempty"`
	// Planned is when the script expected it (its shot), in the clip's seconds.
	Planned float64 `json:"planned"`
	Status  string  `json:"status"`
}

// audioCheck is the detail of a speech check, kept with its verdict.
type audioCheck struct {
	Lines []audioLine `json:"lines"`
	// Heard is everything heard; Extra the speech that belongs to no line.
	Heard string `json:"heard"`
	Extra string `json:"extra,omitempty"`
	// ExpectedLanguage and HeardLanguage are what the script is written in and what
	// was heard, recognized by writing system ("Chinese", "Japanese", ...).
	ExpectedLanguage string   `json:"expected_language,omitempty"`
	HeardLanguage    string   `json:"heard_language,omitempty"`
	SpeechStart      *float64 `json:"speech_start,omitempty"`
	SpeechEnd        *float64 `json:"speech_end,omitempty"`
	ClipLength       float64  `json:"clip_length"`
	CutOff           bool     `json:"cut_off,omitempty"`
	// Listener, Model, Seconds and Cost are who listened, to how much, for how much.
	Listener string  `json:"listener,omitempty"`
	Model    string  `json:"model,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	Cost     float64 `json:"cost,omitempty"`
}

func secondsAt(v float64) *float64 { v = round3(v); return &v }

// judgeClipAudio compares what was heard in a clip with the lines it should say.
// speaker names who spoke ("Omni", "the clip") in the findings.
func judgeClipAudio(lines []timedLine, words []providers.SpokenWord, length float64, speaker string) selfCheckResult {
	detail := &audioCheck{Lines: []audioLine{}, ClipLength: round3(length)}
	result := selfCheckResult{Verdict: verdictPass, Issues: []selfCheckIssue{}, Audio: detail}
	issues := []selfCheckIssue{}
	add := func(code, severity, text string) {
		issues = append(issues, selfCheckIssue{Code: code, Severity: severity, Detail: clip(text, maxSelfCheckDetail)})
	}
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = l.Text
	}
	detail.Heard = clip(joinHeard(words), 1000)
	expected := speechScript(strings.Join(texts, " "))
	if expected == "" && len(lines) > 0 {
		expected = scriptForLanguage(speechCode(lines[0].Language))
	}
	heard := speechScript(detail.Heard)
	if expected != "" {
		detail.ExpectedLanguage = scriptLanguageName(expected)
		if len(lines) > 0 && strings.TrimSpace(lines[0].Language) != "" && expected != scriptLatin {
			detail.ExpectedLanguage = strings.TrimSpace(lines[0].Language)
		}
	}
	if heard != "" {
		detail.HeardLanguage = scriptLanguageName(heard)
	}
	if len(words) > 0 {
		detail.SpeechStart, detail.SpeechEnd = secondsAt(words[0].Start), secondsAt(words[len(words)-1].End)
	}
	matches, extra := alignHeard(texts, words)
	wrongLanguage := len(lines) > 0 && !sameScript(expected, heard)
	lastWord := len(words) - 1
	cutOff := len(words) > 0 && length > 0 && words[lastWord].End >= length-audioCutMargin
	detail.CutOff = false
	total := 0.0
	cutLine := -1
	for i, l := range lines {
		line := audioLine{ID: l.ID, Text: l.Text, Planned: round3(l.Start), Status: audioLineMissing}
		m := matches[i]
		if m.heard() {
			line.Heard = joinHeard(words[m.First : m.Last+1])
			line.Similarity = speechSimilarity(l.Text, line.Heard)
			line.Start, line.End = secondsAt(words[m.First].Start), secondsAt(words[m.Last].End)
			line.Status = audioLineChanged
			if line.Similarity >= audioWordsKept {
				line.Status = audioLineOK
			}
			unfinished := m.TailMissing >= max(1, m.Letters/10) || line.Similarity < audioWordsKept
			if cutOff && m.Last == lastWord && unfinished {
				line.Status, cutLine = audioLineCutOff, i
			}
		}
		total += line.Similarity
		detail.Lines = append(detail.Lines, line)
	}
	// Lines after the one cut off were never reached.
	if cutLine >= 0 {
		for i := cutLine + 1; i < len(detail.Lines); i++ {
			if detail.Lines[i].Status == audioLineMissing {
				detail.Lines[i].Status = audioLineCutOff
			}
		}
	}
	quote := func(text string) string { return "“" + clip(strings.TrimSpace(text), 80) + "”" }
	changes := []string{}
	switch {
	case len(lines) == 0 && len(words) == 0:
		result.Note = "No script lines for this clip and no speech heard."
	case len(words) == 0:
		add("no_speech", "high", "No speech was heard in the clip.")
		changes = append(changes, "Have the characters say every scripted line out loud.")
	case wrongLanguage:
		add("wrong_language", "high", fmt.Sprintf("%s spoke %s, not %s (heard %s).", speaker, detail.HeardLanguage, detail.ExpectedLanguage, quote(detail.Heard)))
		changes = append(changes, fmt.Sprintf("Speak every line in %s only, word for word as written.", detail.ExpectedLanguage))
	}
	if len(words) > 0 && !wrongLanguage {
		reworded := []string{}
		for i, line := range detail.Lines {
			n := i + 1
			switch line.Status {
			case audioLineCutOff:
				if line.Start != nil {
					add("speech_cut_off", "high", fmt.Sprintf("Speech cut off at %.1f s: line %d stops before its end (heard %s).", words[lastWord].End, n, quote(line.Heard)))
				} else {
					add("speech_cut_off", "high", fmt.Sprintf("Speech cut off at %.1f s: line %d (%s) was never reached.", words[lastWord].End, n, quote(line.Text)))
				}
			case audioLineMissing:
				add("line_missing", "high", fmt.Sprintf("Line %d was not heard: %s.", n, quote(line.Text)))
				reworded = append(reworded, quote(line.Text))
			case audioLineChanged:
				severity := "medium"
				if line.Similarity < audioWordsLost {
					severity = "high"
				}
				add("different_words", severity, fmt.Sprintf("%s spoke different words for line %d: heard %s instead of %s (%d%% alike).", speaker, n, quote(line.Heard), quote(line.Text), int(math.Round(line.Similarity*100))))
				reworded = append(reworded, quote(line.Text))
			}
		}
		if cutLine >= 0 {
			detail.CutOff = true
			changes = append(changes, fmt.Sprintf("Give line %d fewer words so the speech is finished by about %.1f s (the clip is %.0f s long), or make the clip longer.", cutLine+1, max(0.5, length-0.6), length))
		} else if cutOff {
			add("speech_at_end", "low", fmt.Sprintf("The last word ends at %.1f s, right at the clip's end (%.1f s); a trim there will cut it.", words[lastWord].End, length))
		}
		if len(reworded) > 0 {
			changes = append(changes, "Have the characters say the scripted lines word for word, in order: "+strings.Join(reworded, " ")+".")
		}
		if len(lines) > 0 && length > 0 && words[0].Start > max(audioLateStartSeconds, audioLateStartShare*length) {
			add("late_start", "low", fmt.Sprintf("Speech starts late, at %.1f s of %.1f s.", words[0].Start, length))
		}
		for i, l := range lines {
			m := matches[i]
			if l.Shot == "" || !m.heard() || m.Guessed || detail.Lines[i].Status != audioLineOK {
				continue
			}
			if at := words[m.First].Start; math.Abs(at-l.Start) > max(audioOffShotSeconds, audioOffShotShare*length) {
				add("line_off_shot", "low", fmt.Sprintf("Line %d is spoken at %.1f s, not in its shot (planned near %.1f s).", i+1, at, l.Start))
			}
		}
		extraWords := []providers.SpokenWord{}
		extraLetters, heardLetters := 0, len(speechLetters(detail.Heard))
		for _, w := range extra {
			extraWords = append(extraWords, words[w])
			extraLetters += len(speechLetters(words[w].Word))
		}
		if len(extraWords) > 0 {
			detail.Extra = clip(joinHeard(extraWords), 400)
		}
		if extraLetters >= 4 && float64(extraLetters) >= audioExtraShare*float64(heardLetters) {
			code := "extra_speech"
			if len(lines) == 0 {
				code = "unexpected_speech"
			}
			add(code, "medium", fmt.Sprintf("Speech that isn't in the script was heard: %s.", quote(detail.Extra)))
			changes = append(changes, "No speech other than the scripted lines.")
		}
	}
	// Score: how much of the script was said, less what makes the clip hard to use.
	score := 100.0
	if len(lines) > 0 {
		score = 100 * total / float64(len(lines))
	}
	worst, cutPenalty := "", false
	for _, issue := range issues {
		switch issue.Severity {
		case "high":
			worst = "high"
		case "medium":
			if worst != "high" {
				worst = "medium"
			}
		case "low":
			score -= 3
		}
		switch issue.Code {
		case "wrong_language", "no_speech":
			score = min(score, 20)
		case "speech_cut_off":
			if !cutPenalty {
				score, cutPenalty = score-20, true
			}
		case "extra_speech", "unexpected_speech":
			score -= 10
		}
	}
	result.Score = int(math.Round(math.Max(0, math.Min(100, score))))
	if len(issues) > maxSelfCheckIssues {
		issues = issues[:maxSelfCheckIssues]
	}
	result.Issues = issues
	switch {
	case hasIssue(issues, "wrong_language") || hasIssue(issues, "no_speech"):
		result.Verdict = verdictFail
	case worst != "":
		result.Verdict = verdictFix
	}
	if len(changes) > 0 && result.Verdict != verdictPass {
		result.Retry = selfCheckRetry{WorthIt: true, Change: clip(strings.Join(changes, " "), maxSelfCheckChange)}
	}
	if result.Verdict != verdictPass {
		result.Suggestion = clip(strings.TrimRight(issues[0].Detail, ".")+". Re-rendering uses video credits; keep the clip if it sounds right to you.", maxSelfCheckSuggestion)
	} else if len(lines) > 0 {
		result.Suggestion = "Every line was heard as written."
	}
	return result
}

func hasIssue(issues []selfCheckIssue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

// audioUnavailable is the speech check's safe default: it never blocks, saying why.
func audioUnavailable(reason string) selfCheckResult {
	r := unavailableResult(reason)
	r.Note = "speech check unavailable: " + clip(strings.TrimSpace(reason), 300)
	return r
}

// errNoListenerCheck is what the speech check says without a listening key.
const errNoListenerCheck = "add an OpenAI or Google Gemini key in AI setup (Listening) to check what is said in clips; it costs about half a cent per minute of video"

// clipAudioTarget is a clip resolved for the speech check: its file and the script
// lines it should say, which (with the video's bytes) are its hash.
func (s *Server) clipAudioTarget(project, id string) (*selfCheckTarget, error) {
	t, err := s.clipTarget(project, id)
	if err != nil {
		return nil, err
	}
	t.Kind, t.References, t.RefNames = selfCheckClipAudio, nil, nil
	t.Label = strings.TrimSuffix(t.Label, " clip") + " clip’s speech"
	var b strings.Builder
	b.WriteString(audioCheckVersion + "\n")
	if plan, e := s.outputScript(project, id); e == nil {
		t.script = plan
		fmt.Fprintf(&b, "length %.3f native %t\n", plan.Length, nativeSound(plan.Job))
		for _, l := range plan.Lines {
			fmt.Fprintf(&b, "%s|%s|%s|%.3f|%s\n", l.ID, l.Shot, l.Language, l.Start, l.Text)
		}
	}
	t.Brief = b.String()
	return t, nil
}

// listenArtifact runs the speech check on a clip: listen (or reuse what was heard)
// and compare. It returns the verdict with the listener and its model.
func (s *Server) listenArtifact(ctx context.Context, t *selfCheckTarget) (selfCheckResult, string, string) {
	plan := t.script
	if plan == nil {
		return audioUnavailable("this clip has no script to compare with"), "", ""
	}
	if !nativeSound(plan.Job) {
		return audioUnavailable("this clip was made without its own sound, so there is no speech to check; its voices come from Voice from script"), "", ""
	}
	texts := make([]string, len(plan.Lines))
	for i, l := range plan.Lines {
		texts[i] = l.Text
	}
	out := plan.Out
	if t.output != nil {
		out = t.output
	}
	hearing, err := s.clipTranscript(ctx, out, plan.Job, strings.Join(texts, " "))
	if err == errNoListener {
		return audioUnavailable(errNoListenerCheck), "", ""
	}
	if err != nil {
		return audioUnavailable("the clip couldn't be listened to: " + err.Error()), s.listener(), ""
	}
	speaker := "The clip"
	if plan.Job.Provider == "gemini" {
		speaker = "Omni"
	}
	result := judgeClipAudio(plan.Lines, hearing.Words, plan.Length, speaker)
	result.Audio.Listener, result.Audio.Model, result.Audio.Seconds = hearing.Listener, hearing.Model, hearing.Seconds
	result.Audio.Cost = math.Round(s.listenCost(hearing.Seconds)*10000) / 10000
	note := fmt.Sprintf("Listened to %.0f s with %s (about $%.3f, paid once per clip; captions reuse it).", hearing.Seconds, listenerName(hearing.Listener), result.Audio.Cost)
	if result.Note != "" {
		note = result.Note + " " + note
	}
	result.Note = note + " Lip-sync is not checked."
	return result, hearing.Listener, hearing.Model
}

func listenerName(name string) string {
	if name == "gemini" {
		return "Google Gemini"
	}
	return "OpenAI"
}
