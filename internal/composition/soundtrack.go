package composition

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// The soundtrack of a film joined from clips: one music track (the bed) from the first
// second to the last, lowered while someone speaks, and short fades where one clip's own
// sound meets the next. Only fields the editor, the preview and the export already know
// are used (volume and edge fades per track), so a saved edit stays an ordinary edit.
//
// Ducking is done with the bed cut into consecutive pieces of the same file at their own
// times in it, so the music never jumps: a piece at the bed level, a piece at the duck
// level while someone speaks, and so on. Where two pieces meet they overlap for Ramp
// seconds, the first fading out while the second fades in, so the level slides instead
// of stepping.

// Mix levels and times, in volume (0-1) and seconds.
const (
	// BedLevel is the music where nobody speaks; DuckLevel is under speech. HyperFrames'
	// sound guide puts music under a voice at about 0.14; 0.35 to 0.14 is about 8 dB.
	BedLevel  = 0.35
	DuckLevel = 0.14
	// DuckAttack is how long before speech the music is already down, DuckRelease how
	// long after it the music stays down, and Ramp how long the slide between the two
	// levels takes.
	DuckAttack  = 0.25
	DuckRelease = 0.4
	Ramp        = 0.3
	// DuckGap is the shortest pause between two lines that lets the music come back up;
	// a shorter one keeps it down, so it does not pump between lines.
	DuckGap = 1.2
	// FilmFadeIn and FilmFadeOut are the bed's fades at the start and end of the film.
	FilmFadeIn  = 0.75
	FilmFadeOut = 1.0
	// CutFade is the fade on each side of a cut between two clips' own sound: their
	// ambience dips for 0.1 s instead of clicking or stopping dead. The bed runs on.
	CutFade = 0.1
	// ShortestBed is the shortest music file that may be looped under a film.
	ShortestBed = 4.0
)

// BedPrefix starts the ids of the bed's pieces, so the bed can be found and replaced.
const BedPrefix = "music-bed-"

// Span is when someone speaks, in film seconds.
type Span struct {
	Start, End float64
}

// Bed is the music track to lay under a film: an asset and its length in seconds.
type Bed struct {
	AssetID string
	Seconds float64
}

// Soundtrack is what Score did, for the edit stage and the assistant.
type Soundtrack struct {
	Music      bool    `json:"music"`
	AssetID    string  `json:"asset_id,omitempty"`
	Continuous bool    `json:"continuous"`
	Pieces     int     `json:"pieces"`
	Ducks      int     `json:"ducks"`
	Looped     bool    `json:"looped,omitempty"`
	CutFades   int     `json:"cut_fades"`
	BedLevel   float64 `json:"bed_level,omitempty"`
	DuckLevel  float64 `json:"duck_level,omitempty"`
}

// Score lays the bed under the film (none when bed is nil) and fades the clips' own
// sound at each cut. A bed laid before is replaced, so scoring again is safe; other
// audio tracks and fades someone set are kept.
func Score(m Manifest, bed *Bed, speech []Span) (Manifest, Soundtrack, error) {
	out := m
	out.Clips = append([]Clip{}, m.Clips...)
	report := Soundtrack{}
	report.CutFades = fadeCuts(out.Clips)
	audio := []Clip{}
	for _, a := range m.Audio {
		if !strings.HasPrefix(a.ID, BedPrefix) {
			audio = append(audio, a)
		}
	}
	out.Audio = audio
	if bed == nil || m.Duration <= 0 {
		return out, report, nil
	}
	if !ValidID(bed.AssetID) {
		return m, report, fmt.Errorf("the music track has an invalid media id")
	}
	if bed.Seconds < ShortestBed {
		return m, report, fmt.Errorf("the music track is %.1f s long; it needs at least %.0f s to run under the film", bed.Seconds, ShortestBed)
	}
	levels := duckLevels(m.Duration, speech)
	pieces := bedPieces(m.Duration, bed.Seconds, levels)
	for i, p := range pieces {
		volume := p.level
		out.Audio = append(out.Audio, Clip{ID: fmt.Sprintf("%s%d", BedPrefix, i+1), AssetID: bed.AssetID, Start: round(p.start), Trim: round(p.trim), Duration: round(p.end - p.start), OriginalDuration: bed.Seconds, Speed: 1, HasAudio: true, Volume: &volume, FadeIn: round(p.fadeIn), FadeOut: round(p.fadeOut)})
	}
	report = Inspect(out)
	report.CutFades = countCutFades(out.Clips)
	return out, report, nil
}

// fadeCuts gives each cut between two clips with their own sound a short fade on both
// sides, where none was set. It returns how many cuts it faded.
func fadeCuts(clips []Clip) int {
	order := make([]int, len(clips))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return clips[order[a]].Start < clips[order[b]].Start })
	n := 0
	for k := 0; k+1 < len(order); k++ {
		a, b := &clips[order[k]], &clips[order[k+1]]
		if !a.HasAudio || !b.HasAudio || a.Muted || b.Muted || a.Overlay != nil || b.Overlay != nil || math.Abs(a.Start+a.Duration-b.Start) > 0.05 {
			continue
		}
		faded := false
		if a.FadeOut == 0 && a.FadeIn+CutFade <= a.Duration/2 {
			a.FadeOut, faded = CutFade, true
		}
		if b.FadeIn == 0 && b.FadeOut+CutFade <= b.Duration/2 {
			b.FadeIn, faded = CutFade, true
		}
		if faded {
			n++
		}
	}
	return n
}

// countCutFades counts cuts between sounding clips with a fade on either side.
func countCutFades(clips []Clip) int {
	sorted := append([]Clip{}, clips...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].Start < sorted[b].Start })
	n := 0
	for k := 0; k+1 < len(sorted); k++ {
		a, b := sorted[k], sorted[k+1]
		if a.HasAudio && b.HasAudio && math.Abs(a.Start+a.Duration-b.Start) <= 0.05 && (a.FadeOut > 0 || b.FadeIn > 0) {
			n++
		}
	}
	return n
}

// level is a stretch of the film at one music level.
type level struct {
	start, end, volume float64
}

// duckLevels cuts the film into stretches at the bed level and the duck level: down from
// DuckAttack before each spoken stretch to DuckRelease after it, with pauses shorter than
// DuckGap kept down.
func duckLevels(duration float64, speech []Span) []level {
	spans := []Span{}
	for _, s := range speech {
		start, end := math.Max(0, s.Start-DuckAttack), math.Min(duration, s.End+DuckRelease)
		if end > start {
			spans = append(spans, Span{start, end})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	merged := []Span{}
	for _, s := range spans {
		if n := len(merged); n > 0 && s.Start-merged[n-1].End < DuckGap {
			merged[n-1].End = math.Max(merged[n-1].End, s.End)
			continue
		}
		merged = append(merged, s)
	}
	// A loud stretch at either end shorter than a pause is kept down too.
	if len(merged) > 0 && merged[0].Start < DuckGap {
		merged[0].Start = 0
	}
	if n := len(merged); n > 0 && duration-merged[n-1].End < DuckGap {
		merged[n-1].End = duration
	}
	out, at := []level{}, 0.0
	for _, s := range merged {
		if s.Start > at {
			out = append(out, level{at, s.Start, BedLevel})
		}
		out = append(out, level{s.Start, s.End, DuckLevel})
		at = s.End
	}
	if at < duration {
		out = append(out, level{at, duration, BedLevel})
	}
	return out
}

// piece is one audio track of the bed.
type piece struct {
	start, end, trim, level, fadeIn, fadeOut float64
}

// bedPieces lays the music over the levels: each level stretch is a piece, overlapping
// the next by Ramp so one fades out as the other fades in, and the file's time follows
// the film's (looping when the file is shorter), so the music runs on unbroken.
func bedPieces(duration, length float64, levels []level) []piece {
	logical := []piece{}
	for i, l := range levels {
		p := piece{start: l.start, end: l.end, level: l.volume}
		if i > 0 {
			p.start -= Ramp / 2
			p.fadeIn = Ramp
		}
		if i < len(levels)-1 {
			p.end += Ramp / 2
			p.fadeOut = Ramp
		}
		logical = append(logical, p)
	}
	if n := len(logical); n > 0 {
		logical[0].fadeIn = math.Min(FilmFadeIn, (logical[0].end-logical[0].start)/2)
		last := &logical[n-1]
		last.fadeOut = math.Min(FilmFadeOut, (last.end-last.start)/2)
		if n == 1 {
			last.fadeIn = math.Min(FilmFadeIn, (last.end-last.start)/2)
		}
	}
	// A piece that crosses the end of the file is split there: the file starts over.
	out := []piece{}
	for _, p := range logical {
		start := p.start
		for start < p.end-1e-6 {
			loop := math.Floor(start/length+1e-9) * length
			end := math.Min(p.end, loop+length)
			q := piece{start: start, end: end, trim: start - loop, level: p.level}
			if start == p.start {
				q.fadeIn = p.fadeIn
			}
			if end == p.end {
				q.fadeOut = p.fadeOut
			}
			// Fades never pass the piece they are on.
			span := q.end - q.start
			if q.fadeIn+q.fadeOut > span {
				scale := span / (q.fadeIn + q.fadeOut)
				q.fadeIn, q.fadeOut = math.Floor(q.fadeIn*scale*1000)/1000, math.Floor(q.fadeOut*scale*1000)/1000
			}
			out = append(out, q)
			start = end
		}
	}
	return out
}

// Inspect says what music an edit has: the bed's pieces, whether they run from the
// first second to the last without a gap, and whether they duck.
func Inspect(m Manifest) Soundtrack {
	r := Soundtrack{}
	pieces := []Clip{}
	for _, a := range m.Audio {
		if strings.HasPrefix(a.ID, BedPrefix) && !a.Muted {
			pieces = append(pieces, a)
		}
	}
	r.CutFades = countCutFades(m.Clips)
	if len(pieces) == 0 {
		return r
	}
	sort.SliceStable(pieces, func(i, j int) bool { return pieces[i].Start < pieces[j].Start })
	r.Music, r.AssetID, r.Pieces = true, pieces[0].AssetID, len(pieces)
	r.BedLevel, r.DuckLevel = 0, 1
	reach, continuous := 0.0, true
	for i, p := range pieces {
		continuous = continuous && p.Start <= reach+0.01 && p.AssetID == r.AssetID
		reach = math.Max(reach, p.Start+p.Duration)
		v := clipLevel(p)
		r.BedLevel, r.DuckLevel = math.Max(r.BedLevel, v), math.Min(r.DuckLevel, v)
		if i > 0 && v < clipLevel(pieces[i-1]) {
			r.Ducks++
		}
		r.Looped = r.Looped || (i > 0 && p.Trim < pieces[i-1].Trim)
	}
	if len(pieces) > 0 && clipLevel(pieces[0]) < r.BedLevel {
		r.Ducks++
	}
	r.Continuous = continuous && reach >= m.Duration-0.01
	return r
}

func clipLevel(c Clip) float64 {
	if c.Volume == nil {
		return 1
	}
	return *c.Volume
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }
