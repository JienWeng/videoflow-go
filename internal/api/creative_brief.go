package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"videoflow-go/internal/agents"
)

func creativeStyleFingerprint(style any) string {
	hash := sha256.Sum256(voiceJSON(style))
	return hex.EncodeToString(hash[:])
}
func mergeCreativeBrief(previous, next *agents.CreativeBrief, message string) agents.CreativeBrief {
	result := agents.CreativeBrief{Idea: message}
	if previous != nil {
		result = *previous
	}
	if next == nil {
		return result
	}
	if next.Idea != "" {
		result.Idea = next.Idea
	}
	if next.PresetID != "" && next.PresetID != result.PresetID {
		result.PresetID = next.PresetID
		result.StyleFingerprint = ""
	}
	for _, pair := range []struct {
		dst *string
		src string
	}{{&result.StorytellingID, next.StorytellingID}, {&result.Instruction, next.Instruction}, {&result.Tone, next.Tone}, {&result.Audience, next.Audience}, {&result.Platform, next.Platform}, {&result.AspectRatio, next.AspectRatio}, {&result.Resolution, next.Resolution}, {&result.Language, next.Language}, {&result.ConversationMode, next.ConversationMode}} {
		if strings.TrimSpace(pair.src) != "" {
			*pair.dst = strings.TrimSpace(pair.src)
		}
	}
	// The brief form's language menu uses names, so a code such as "en" would leave it blank.
	result.Language = languageLabel(result.Language)
	if next.SceneCount > 0 {
		result.SceneCount = next.SceneCount
	}
	if next.TargetDuration > 0 {
		result.TargetDuration = next.TargetDuration
	}
	return result
}

var languageLabels = map[string]string{"en": "English", "es": "Spanish", "zh": "Mandarin", "ta": "Tamil", "ja": "Japanese", "ko": "Korean", "fr": "French", "de": "German", "pt": "Portuguese", "it": "Italian", "ru": "Russian", "ar": "Arabic", "hi": "Hindi", "id": "Indonesian", "ms": "Malay", "th": "Thai", "vi": "Vietnamese", "tr": "Turkish", "nl": "Dutch"}

// languageLabel shows a language code such as "en" or "zh-CN" as a name; other text is kept.
func languageLabel(v string) string {
	code := strings.ToLower(strings.TrimSpace(v))
	if base, _, found := strings.Cut(strings.ReplaceAll(code, "_", "-"), "-"); found {
		code = base
	}
	if name, ok := languageLabels[code]; ok {
		return name
	}
	return v
}

// fillCreativeBriefDefaults decides the mechanical choices so the user is not asked for
// what follows from what they already said: the format from the platform, a scene count
// that fits the video model's clip length, and the project's own look.
func fillCreativeBriefDefaults(b *agents.CreativeBrief, projectHasStyle bool) {
	platform := strings.ToLower(b.Platform)
	if b.AspectRatio == "" {
		switch {
		case strings.Contains(platform, "short"), strings.Contains(platform, "tiktok"), strings.Contains(platform, "reel"), strings.Contains(platform, "douyin"), strings.Contains(platform, "xiaohongshu"), strings.Contains(platform, "story"):
			b.AspectRatio = "9:16"
		case strings.Contains(platform, "youtube"), strings.Contains(platform, "website"), strings.Contains(platform, "linkedin"), strings.Contains(platform, "bilibili"), strings.Contains(platform, "tv"):
			b.AspectRatio = "16:9"
		}
	}
	if b.Platform == "" {
		switch b.AspectRatio {
		case "9:16":
			b.Platform = "TikTok"
		case "16:9":
			b.Platform = "YouTube"
		}
	}
	if b.Resolution == "" && (b.Platform != "" || b.AspectRatio != "") {
		b.Resolution = "720p"
	}
	if b.PresetID == "" && projectHasStyle {
		b.PresetID = "project-style"
	}
	if b.TargetDuration > 0 {
		// One scene is one clip of at most about 10 to 15 seconds on current video models.
		if fit := (b.TargetDuration + 9) / 10; b.SceneCount <= 0 || (b.TargetDuration+b.SceneCount-1)/b.SceneCount > 12 {
			b.SceneCount = min(20, max(1, fit))
		}
	} else if b.SceneCount > 0 {
		b.TargetDuration = b.SceneCount * 8
	}
}

// creativeBriefQuestion asks for what is still missing, after echoing what was understood,
// so a follow-up like "English, 15 seconds" visibly lands instead of the same question repeating.
func creativeBriefQuestion(b agents.CreativeBrief) string {
	question := creativeBriefNextQuestion(b)
	got := []string{}
	for _, v := range []string{b.Tone, b.Audience, b.Platform, b.AspectRatio, languageLabel(b.Language)} {
		if strings.TrimSpace(v) != "" {
			got = append(got, v)
		}
	}
	if b.TargetDuration > 0 {
		got = append(got, fmt.Sprintf("%d seconds", b.TargetDuration))
	}
	if b.SceneCount > 0 {
		got = append(got, plural(b.SceneCount, "scene"))
	}
	if len(got) == 0 || strings.HasPrefix(question, "Please confirm") {
		return question
	}
	return "Got it: " + strings.Join(got, ", ") + ". " + question
}

func creativeBriefNextQuestion(b agents.CreativeBrief) string {
	look := []string{}
	if b.PresetID == "" {
		look = append(look, "visual style")
	}
	if b.Tone == "" {
		look = append(look, "tone")
	}
	if b.Audience == "" {
		look = append(look, "intended audience")
	}
	if len(look) > 0 {
		return "Before I create the story, please choose the " + joinAnd(look) + ". You can answer here or use the choices below."
	}
	delivery := []string{}
	for _, field := range []struct{ value, label string }{{b.Platform, "destination platform"}, {b.AspectRatio, "format"}, {b.Resolution, "video size"}, {b.Language, "spoken language"}, {b.ConversationMode, "speech (dialogue, narration or none)"}} {
		if field.value == "" {
			delivery = append(delivery, field.label)
		}
	}
	if len(delivery) > 0 {
		return "What would you like for the " + joinAnd(delivery) + "?"
	}
	if b.TargetDuration <= 0 && b.SceneCount <= 0 {
		return "How long should the complete video be, and how many scenes would you like? A scene can contain several shots."
	}
	if b.SceneCount <= 0 {
		return fmt.Sprintf("How many scenes would you like within the %d-second video? Scenes are story units, not automatically separate paid video calls.", b.TargetDuration)
	}
	if b.TargetDuration <= 0 {
		return fmt.Sprintf("How long should the whole video be across %s?", plural(b.SceneCount, "scene"))
	}
	return fmt.Sprintf("Please confirm the brief below: %s tone, %s audience, %s, %s at %s, %s %s, %d seconds across %s. Check the visual style too. Nothing generates until you confirm and choose Create.", b.Tone, b.Audience, b.Platform, b.AspectRatio, b.Resolution, b.Language, b.ConversationMode, b.TargetDuration, plural(b.SceneCount, "scene"))
}
func (s *Server) confirmedCreativeBrief(project string, b *generationBrief) error {
	if b.BriefVersion == 0 {
		return nil
	}
	if b.BriefVersion != 1 {
		return fmt.Errorf("unsupported creative brief version")
	}
	if !b.BriefConfirmed {
		return fmt.Errorf("confirm the complete creative brief before creating the story")
	}
	for name, value := range map[string]string{"visual style": b.PresetID, "tone": b.Tone, "audience": b.Audience, "platform": b.Platform, "format": b.AspectRatio, "resolution": b.Resolution, "language": b.Language, "dialogue mode": b.ConversationMode} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("choose %s in the creative brief", name)
		}
	}
	if b.SceneCount < 1 || b.SceneCount > 20 || b.TargetDuration < b.SceneCount || b.TargetDuration > 600 {
		return fmt.Errorf("confirm 1–20 scenes and a total duration between the scene count and 600 seconds")
	}
	if len(b.Tone) > 200 || len(b.Audience) > 200 || len(b.Platform) > 120 || len(b.Language) > 80 {
		return fmt.Errorf("creative brief values are too long")
	}
	switch b.AspectRatio {
	case "9:16", "16:9", "1:1", "3:4", "4:3", "21:9":
	default:
		return fmt.Errorf("choose a supported video format")
	}
	switch b.Resolution {
	case "auto", "480p", "720p":
	default:
		return fmt.Errorf("choose model default, 480p or 720p resolution")
	}
	var current string
	if b.PresetID == "project-style" {
		style, e := s.database.GetStyleGuide(project)
		if e != nil || style == nil || strings.TrimSpace(style.StylePrompt) == "" {
			return fmt.Errorf("project style is unavailable; choose a style again")
		}
		current = creativeStyleFingerprint(style)
	} else {
		p, e := s.findPreset(b.PresetID)
		if e != nil {
			return e
		}
		current = creativeStyleFingerprint(p)
	}
	if b.StyleFingerprint == "" || current != b.StyleFingerprint {
		return fmt.Errorf("the selected style changed; choose it again and reconfirm the brief")
	}
	return nil
}

// Confirmed totals and format are authoritative; model timing estimates are not.
func applyConfirmedDraft(b generationBrief, draft *agents.ScriptDraft) error {
	if b.BriefVersion != 1 {
		return nil
	}
	if len(draft.Scenes) != b.SceneCount {
		return fmt.Errorf("planner returned a different scene count; the confirmed brief was not changed")
	}
	if b.TargetDuration < len(draft.Scenes) {
		return fmt.Errorf("confirmed duration cannot fit the scene count")
	}
	total := 0
	for _, scene := range draft.Scenes {
		if scene.Duration < 1 || scene.Duration > 3600 {
			return fmt.Errorf("planner returned an invalid scene duration")
		}
		total += scene.Duration
	}
	remaining := b.TargetDuration - len(draft.Scenes)
	used := 0
	fractions := make([]float64, len(draft.Scenes))
	for i := range draft.Scenes {
		ideal := float64(draft.Scenes[i].Duration) * float64(remaining) / float64(total)
		extra := int(ideal)
		draft.Scenes[i].Duration = 1 + extra
		used += 1 + extra
		fractions[i] = ideal - float64(extra)
		draft.Scenes[i].AspectRatio = b.AspectRatio
		if b.ConversationMode == "silent" {
			draft.Scenes[i].SpokenLines = nil
		} else {
			for j := range draft.Scenes[i].SpokenLines {
				draft.Scenes[i].SpokenLines[j].Language = b.Language
				if b.ConversationMode == "narration" {
					draft.Scenes[i].SpokenLines[j].Visible = false
				}
			}
		}
	}
	for used < b.TargetDuration {
		best := 0
		for i := range fractions {
			if fractions[i] > fractions[best] {
				best = i
			}
		}
		draft.Scenes[best].Duration++
		fractions[best] = -1
		used++
	}
	return nil
}

func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
