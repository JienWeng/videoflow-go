package api

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"videoflow-go/internal/composition"
)

// The AI editor proposes a whole edit; these checks decide whether VideoFlow keeps it.
// They come on top of the edit file's own validation (composition.Save): the speech
// the scenes were made with must still play whole, at its own timing, with its
// captions on the times the words are heard.

// Tolerances, in seconds. Edit times are kept to milliseconds.
const (
	aiEditMediaSlack   = 0.02 // a clip may end this far past the probed media length
	aiEditLineSlack    = 0.02 // a covering clip may start or end this far inside a line
	aiEditCaptionSlack = 0.05 // a caption may sit this far off its line
	aiEditSpeechVolume = 0.5  // the least volume a clip speaking a line keeps
	aiEditUnderSpeech  = 0.5  // the most volume a new or changed track has under a line
)

// spokenLine is a line said in the current edit: a caption anchored to the clip that
// speaks it, with where it is in that clip's own media.
type spokenLine struct {
	CaptionID   string  `json:"caption_id"`
	UtteranceID string  `json:"utterance_id,omitempty"`
	Text        string  `json:"text"`
	Timing      string  `json:"timing,omitempty"`
	Track       string  `json:"track"` // clips or audio
	ClipID      string  `json:"clip_id"`
	AssetID     string  `json:"asset_id"`
	MediaStart  float64 `json:"media_start"`
	MediaEnd    float64 `json:"media_end"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
}

// isSpeechCaption says whether a caption shows a line spoken in the film (a script
// caption, timed by hearing, by estimate or by a voice line), not a free note.
func isSpeechCaption(c composition.Cue) bool {
	return c.UtteranceID != "" || c.Timing == timingHeard || c.Timing == timingEstimated || c.Timing == timingVoice
}

// spokenLines finds, for each speech caption of an edit, the clip that speaks it: a
// voice line's audio clip, otherwise the full-frame picture clip under it.
func spokenLines(m composition.Manifest) []spokenLine {
	lines := []spokenLine{}
	holds := func(c composition.Clip, cue composition.Cue) bool {
		return cue.Start >= c.Start-0.01 && cue.End <= c.Start+c.Duration+0.01
	}
	for _, cue := range m.Captions {
		if !isSpeechCaption(cue) {
			continue
		}
		var owner *composition.Clip
		track := "clips"
		if cue.Timing == timingVoice {
			for i := range m.Audio {
				if holds(m.Audio[i], cue) {
					owner, track = &m.Audio[i], "audio"
					break
				}
			}
		}
		if owner == nil {
			for i := range m.Clips {
				if m.Clips[i].Overlay == nil && holds(m.Clips[i], cue) {
					owner, track = &m.Clips[i], "clips"
					break
				}
			}
		}
		if owner == nil {
			continue // a caption over no clip is a note; it is checked as any caption is
		}
		lines = append(lines, spokenLine{
			CaptionID: cue.ID, UtteranceID: cue.UtteranceID, Text: cue.Text, Timing: cue.Timing, Track: track,
			ClipID: owner.ID, AssetID: owner.AssetID,
			MediaStart: round3(owner.Trim + cue.Start - owner.Start), MediaEnd: round3(owner.Trim + cue.End - owner.Start),
			Start: cue.Start, End: cue.End,
		})
	}
	return lines
}

// aiEditBounds is how short and how long a proposal may make the film.
func aiEditBounds(base composition.Manifest) (float64, float64) {
	return round3(math.Max(1, base.Duration*0.5)), round3(math.Min(600, base.Duration*1.5))
}

func clipVolume(c composition.Clip) float64 {
	if c.Volume == nil {
		return 1
	}
	return *c.Volume
}

func overlaps(aStart, aEnd, bStart, bEnd float64) bool {
	return aStart < bEnd-0.01 && aEnd > bStart+0.01
}

// sameTiming says whether a clip is in a proposal exactly as it was in the edit.
func sameTiming(a, b composition.Clip) bool {
	return a.AssetID == b.AssetID && a.Start == b.Start && a.Trim == b.Trim && a.Duration == b.Duration && clipVolume(a) == clipVolume(b) && a.Muted == b.Muted && (a.Overlay == nil) == (b.Overlay == nil)
}

func quoteLine(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) > 60 {
		text = string([]rune(text)[:57]) + "…"
	}
	return "“" + text + "”"
}

// checkAIEditProposal lists every reason a proposal is not kept; none means it may be
// saved. mediaSeconds is each asset's probed length (0 for a still), allowed the media
// the proposal may use (the project's and the session's new files).
func checkAIEditProposal(base, proposal composition.Manifest, mediaSeconds map[string]float64, allowed map[string]bool) []string {
	reasons := []string{}
	add := func(format string, args ...any) { reasons = append(reasons, fmt.Sprintf(format, args...)) }
	if proposal.Version != 1 {
		add("the proposal is not a version-1 edit")
	}
	if proposal.Width != base.Width || proposal.Height != base.Height || proposal.FPS != base.FPS {
		add("format changed: the film must stay %dx%d at %d fps", base.Width, base.Height, base.FPS)
	}
	low, high := aiEditBounds(base)
	if proposal.Duration < low || proposal.Duration > high {
		add("length out of bounds: %.2f s, but the film must stay between %.2f s and %.2f s", proposal.Duration, low, high)
	}
	// Media: only the project's own, within its length, at normal speed.
	for _, track := range [][]composition.Clip{proposal.Clips, proposal.Audio} {
		for _, c := range track {
			if !allowed[c.AssetID] {
				add("clip %s uses media %q that is not in this project", c.ID, c.AssetID)
				continue
			}
			if c.Speed != 1 {
				add("clip %s plays at speed %v; only normal speed keeps the speech timing", c.ID, c.Speed)
			}
			if length, ok := mediaSeconds[c.AssetID]; ok && length > 0 && c.Trim+c.Duration > length+aiEditMediaSlack {
				add("media exceeded: clip %s uses %.2f s of its media (from %.2f s for %.2f s) but the media is %.2f s long", c.ID, c.Trim+c.Duration, c.Trim, c.Duration, length)
			}
		}
	}
	baseClips := map[string]composition.Clip{}
	for _, c := range append(append([]composition.Clip{}, base.Clips...), base.Audio...) {
		baseClips[c.ID] = c
	}
	unchanged := func(c composition.Clip) bool {
		before, ok := baseClips[c.ID]
		return ok && sameTiming(before, c)
	}
	captions := map[string]composition.Cue{}
	for _, cue := range proposal.Captions {
		captions[cue.ID] = cue
	}
	for _, line := range spokenLines(base) {
		track := proposal.Clips
		if line.Track == "audio" {
			track = proposal.Audio
		}
		// The clips that still play this line whole, and where the line falls in each.
		type placement struct {
			index      int
			start, end float64
		}
		covering, muted := []placement{}, false
		for i, c := range track {
			if c.AssetID != line.AssetID || (line.Track == "clips" && c.Overlay != nil) {
				continue
			}
			if c.Trim > line.MediaStart+aiEditLineSlack || c.Trim+c.Duration < line.MediaEnd-aiEditLineSlack {
				continue
			}
			if c.Muted || clipVolume(c) < aiEditSpeechVolume {
				muted = true
				continue
			}
			covering = append(covering, placement{i, round3(c.Start + line.MediaStart - c.Trim), round3(c.Start + line.MediaEnd - c.Trim)})
		}
		if len(covering) == 0 {
			if muted {
				add("speech muted: the line %s (clip %s) is turned down below %.1f or muted", quoteLine(line.Text), line.ClipID, aiEditSpeechVolume)
			} else {
				add("speech cut: the line %s (clip %s, %.2f-%.2f s of its media) no longer plays whole", quoteLine(line.Text), line.ClipID, line.MediaStart, line.MediaEnd)
			}
			continue
		}
		cue, ok := captions[line.CaptionID]
		if !ok {
			add("caption removed: the caption of the line %s is gone", quoteLine(line.Text))
			continue
		}
		if strings.TrimSpace(cue.Text) != strings.TrimSpace(line.Text) {
			add("caption changed: the caption of the line %s has new words", quoteLine(line.Text))
		}
		var at *placement
		for i := range covering {
			if math.Abs(cue.Start-covering[i].start) <= aiEditCaptionSlack && math.Abs(cue.End-covering[i].end) <= aiEditCaptionSlack {
				at = &covering[i]
				break
			}
		}
		if at == nil {
			add("caption moved off the heard time: the line %s is spoken at %.2f-%.2f s but its caption shows at %.2f-%.2f s", quoteLine(line.Text), covering[0].start, covering[0].end, cue.Start, cue.End)
			at = &covering[0]
		}
		// Nothing new may hide the speaker or drown the line.
		if line.Track == "clips" {
			for i := at.index + 1; i < len(proposal.Clips); i++ {
				c := proposal.Clips[i]
				if c.Overlay == nil && !unchanged(c) && overlaps(c.Start, c.Start+c.Duration, at.start, at.end) && (c.Adjustments == nil || c.Adjustments.Opacity > 0) {
					add("speaker hidden: full-frame clip %s covers the line %s; make it an overlay or move it off the line", c.ID, quoteLine(line.Text))
				}
			}
		}
		for _, c := range proposal.Audio {
			if line.Track == "audio" && c.AssetID == line.AssetID {
				continue
			}
			if !unchanged(c) && !c.Muted && clipVolume(c) > aiEditUnderSpeech && overlaps(c.Start, c.Start+c.Duration, at.start, at.end) {
				add("sound too loud under speech: track %s plays at volume %.2f during the line %s (at most %.1f)", c.ID, clipVolume(c), quoteLine(line.Text), aiEditUnderSpeech)
			}
		}
	}
	return reasons
}

// carryCaptionVariants keeps the edit's translated captions and moves each with the
// caption it translates (they share its id).
func carryCaptionVariants(base, proposal composition.Manifest) []composition.CaptionVariant {
	before, after := map[string]composition.Cue{}, map[string]composition.Cue{}
	for _, c := range base.Captions {
		before[c.ID] = c
	}
	for _, c := range proposal.Captions {
		after[c.ID] = c
	}
	variants := []composition.CaptionVariant{}
	for _, v := range base.CaptionVariants {
		moved := composition.CaptionVariant{Language: v.Language, Captions: []composition.Cue{}}
		for _, cue := range v.Captions {
			if b, ok := before[cue.ID]; ok {
				a, kept := after[cue.ID]
				if !kept {
					continue // its caption was dropped (the check says why)
				}
				shift := a.Start - b.Start
				cue.Start, cue.End = round3(cue.Start+shift), round3(cue.End+shift)
			}
			moved.Captions = append(moved.Captions, cue)
		}
		variants = append(variants, moved)
	}
	return variants
}

// aiEditDiff says in words what a proposal changes, for the owner to judge it.
type aiEditDiff struct {
	Lines         []string `json:"lines"`
	Reordered     bool     `json:"reordered"`
	Cuts          int      `json:"cuts"`
	Removed       int      `json:"removed"`
	TitlesAdded   int      `json:"titles_added"`
	SoundsAdded   int      `json:"sounds_added"`
	OverlaysAdded int      `json:"overlays_added"`
	VolumeChanges int      `json:"volume_changes"`
	LengthBefore  float64  `json:"length_before"`
	LengthAfter   float64  `json:"length_after"`
}

func describeAIEdit(base, proposal composition.Manifest, newMedia map[string]aiEditMedia) aiEditDiff {
	d := aiEditDiff{Lines: []string{}, LengthBefore: base.Duration, LengthAfter: proposal.Duration}
	order := func(clips []composition.Clip, keep map[string]bool) []string {
		sorted := append([]composition.Clip{}, clips...)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
		ids := []string{}
		for _, c := range sorted {
			if keep[c.ID] {
				ids = append(ids, c.ID)
			}
		}
		return ids
	}
	inBoth := map[string]bool{}
	before := map[string]composition.Clip{}
	for _, c := range append(append([]composition.Clip{}, base.Clips...), base.Audio...) {
		before[c.ID] = c
	}
	after := map[string]bool{}
	for _, c := range append(append([]composition.Clip{}, proposal.Clips...), proposal.Audio...) {
		after[c.ID] = true
		b, ok := before[c.ID]
		if !ok {
			if m, isNew := newMedia[c.AssetID]; isNew {
				switch {
				case m.Kind == "title":
					d.TitlesAdded++
				case strings.HasPrefix(m.MediaKind, "audio"):
					d.SoundsAdded++
				default:
					d.OverlaysAdded++
				}
			} else if c.Overlay != nil {
				d.OverlaysAdded++
			}
			continue
		}
		inBoth[c.ID] = true
		if b.Trim != c.Trim || b.Duration != c.Duration {
			d.Cuts++
		}
		if clipVolume(b) != clipVolume(c) || b.Muted != c.Muted || b.FadeIn != c.FadeIn || b.FadeOut != c.FadeOut {
			d.VolumeChanges++
		}
	}
	for id := range before {
		if !after[id] {
			d.Removed++
		}
	}
	d.Reordered = strings.Join(order(base.Clips, inBoth), ",") != strings.Join(order(proposal.Clips, inBoth), ",")
	baseTexts := map[string]bool{}
	for _, t := range base.Texts {
		baseTexts[t.ID] = true
	}
	for _, t := range proposal.Texts {
		if !baseTexts[t.ID] {
			d.TitlesAdded++
		}
	}
	if d.Reordered {
		d.Lines = append(d.Lines, "Scenes put in a new order")
	}
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	if d.Cuts > 0 {
		d.Lines = append(d.Lines, plural(d.Cuts, "clip trimmed", "clips trimmed"))
	}
	if d.Removed > 0 {
		d.Lines = append(d.Lines, plural(d.Removed, "clip removed", "clips removed"))
	}
	if d.TitlesAdded > 0 {
		d.Lines = append(d.Lines, plural(d.TitlesAdded, "title added", "titles added"))
	}
	if d.OverlaysAdded > 0 {
		d.Lines = append(d.Lines, plural(d.OverlaysAdded, "motion overlay added", "motion overlays added"))
	}
	if d.SoundsAdded > 0 {
		d.Lines = append(d.Lines, plural(d.SoundsAdded, "sound added", "sounds added"))
	}
	if d.VolumeChanges > 0 {
		d.Lines = append(d.Lines, plural(d.VolumeChanges, "volume or fade changed", "volumes or fades changed"))
	}
	if base.CaptionStyle != proposal.CaptionStyle && proposal.CaptionStyle != "" {
		d.Lines = append(d.Lines, "Caption style: "+proposal.CaptionStyle)
	}
	if math.Abs(base.Duration-proposal.Duration) >= 0.05 {
		d.Lines = append(d.Lines, fmt.Sprintf("Length %.1f s → %.1f s", base.Duration, proposal.Duration))
	}
	if len(d.Lines) == 0 {
		d.Lines = append(d.Lines, "Small adjustments only")
	}
	return d
}
