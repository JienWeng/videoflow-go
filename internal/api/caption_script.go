package api

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

type SpokenLine struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	Language    string `json:"language"`
	Text        string `json:"text"`
	Visible     bool   `json:"visible"`
}
type SpokenScript struct {
	Approved bool         `json:"approved"`
	Lines    []SpokenLine `json:"lines"`
}
type VoiceProfile struct {
	CharacterID   string `json:"character_id"`
	Mode          string `json:"mode"` // saved (default) or native_reference
	SampleAssetID string `json:"sample_asset_id"`
	SampleHash    string `json:"sample_hash"`
	VoiceID       string `json:"voice_id"`
	Approved      bool   `json:"approved"`
}

func alignmentLetters(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// Provider timing is the only timing source. Exact approved line text is the only
// wording source. A mismatch is reviewable data, never invented timing.
func alignSpokenLines(lines []SpokenLine, raw json.RawMessage) ([]models.CaptionSegment, error) {
	var result struct {
		Words []struct {
			Text  string  `json:"text"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"words"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result.Words) == 0 {
		return nil, fmt.Errorf("alignment has no timed words; review audio/script drift")
	}
	var actual strings.Builder
	starts, ends := []float64{}, []float64{}
	previous := 0.0
	for _, word := range result.Words {
		if math.IsNaN(word.Start) || math.IsNaN(word.End) || math.IsInf(word.Start, 0) || math.IsInf(word.End, 0) || word.Start < 0 || word.End <= word.Start || word.Start < previous {
			return nil, fmt.Errorf("invalid or overlapping alignment timing")
		}
		previous = word.End
		letters := alignmentLetters(word.Text)
		actual.WriteString(letters)
		for range []rune(letters) {
			starts = append(starts, word.Start)
			ends = append(ends, word.End)
		}
	}
	var expected strings.Builder
	for _, line := range lines {
		expected.WriteString(alignmentLetters(line.Text))
	}
	if actual.String() != expected.String() {
		return nil, fmt.Errorf("alignment wording differs from approved script; review omissions/drift before captions")
	}
	out := []models.CaptionSegment{}
	offset := 0
	for _, line := range lines {
		length := len([]rune(alignmentLetters(line.Text)))
		if length == 0 {
			return nil, fmt.Errorf("spoken line %s has no alignable wording", line.ID)
		}
		out = append(out, models.CaptionSegment{ID: line.ID, CharacterID: line.CharacterID, Language: line.Language, Text: line.Text, Start: starts[offset], End: ends[offset+length-1]})
		offset += length
	}
	return out, nil
}
func exportScriptCaptions(cues []models.CaptionSegment, format string) (string, error) {
	if format == "json" {
		raw, err := json.MarshalIndent(map[string]any{"segments": cues}, "", "  ")
		return string(raw), err
	}
	if format != "srt" && format != "vtt" {
		return "", fmt.Errorf("format must be srt, vtt or json")
	}
	var out strings.Builder
	if format == "vtt" {
		out.WriteString("WEBVTT\n\n")
	}
	clock := func(seconds float64) string {
		ms := int64(math.Round(seconds * 1000))
		sep := ","
		if format == "vtt" {
			sep = "."
		}
		return fmt.Sprintf("%02d:%02d:%02d%s%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, sep, ms%1000)
	}
	for i, cue := range cues {
		if cue.Start < 0 || cue.End <= cue.Start || math.IsNaN(cue.Start) || math.IsNaN(cue.End) || math.IsInf(cue.Start, 0) || math.IsInf(cue.End, 0) {
			return "", fmt.Errorf("invalid caption timing")
		}
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", i+1, clock(cue.Start), clock(cue.End), strings.ReplaceAll(cue.Text, "\r", ""))
	}
	return out.String(), nil
}

// ValidateScriptCaptionSource rejects reuse after spoken-script, reference or
// final-audio changes. The editor may show stale cues for repair; burn/export
// must call this before producing a production artifact.
func (s *Server) ValidateScriptCaptionSource(output *models.RenderOutput) error {
	var meta struct {
		Bindings []db.VoiceBinding `json:"source_bindings"`
	}
	if json.Unmarshal(output.CaptionsJSON, &meta) != nil || len(meta.Bindings) == 0 {
		return nil
	}
	job, err := s.database.GetRenderJob(output.RenderJobID)
	if err != nil || job.ProjectID == nil {
		return fmt.Errorf("caption source project is unavailable")
	}
	return s.checkVoiceBindings(&db.VoiceTask{ProjectID: *job.ProjectID, Bindings: meta.Bindings})
}

// ScriptCaptionPayload preserves alignment provenance through the existing
// caption editor. Editing cue text/style does not erase its source revisions.
func (s *Server) ScriptCaptionPayload(output *models.RenderOutput, segments []models.CaptionSegment, style string) json.RawMessage {
	var payload map[string]any
	if json.Unmarshal(output.CaptionsJSON, &payload) != nil || payload == nil {
		payload = map[string]any{}
	}
	payload["segments"] = segments
	payload["style"] = style
	return voiceJSON(payload)
}
