package composition

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// MixAudio exports only the current editable mix, respecting trims, gain, fades and placement.
func MixAudio(ctx context.Context, m Manifest, sources map[string]string, destination string) error {
	if !(m.Duration > 0 && m.Duration <= 600) || len(m.Clips)+len(m.Audio) > 64 {
		return fmt.Errorf("mix must contain at most 64 clips and last at most 600 seconds")
	}
	args := []string{"-v", "error", "-nostdin", "-y"}
	filters := []string{}
	labels := []string{}
	for _, clip := range append(append([]Clip{}, m.Clips...), m.Audio...) {
		path, ok := sources[clip.AssetID]
		if !ok {
			return fmt.Errorf("audio source is not in this project")
		}
		if clip.Speed != 1 || !(clip.Start >= 0 && clip.Trim >= 0 && clip.Duration > 0 && clip.Start+clip.Duration <= m.Duration) || clip.FadeIn < 0 || clip.FadeOut < 0 || clip.FadeIn+clip.FadeOut > clip.Duration {
			return fmt.Errorf("invalid audio clip timings")
		}
		duration, _, _, audio, e := Probe(ctx, path)
		if e != nil {
			return e
		}
		if duration > 0 && clip.Trim+clip.Duration > duration+.02 {
			return fmt.Errorf("audio trim exceeds source")
		}
		if !audio || clip.Muted {
			continue
		}
		gain := 1.0
		if clip.Volume != nil {
			gain = *clip.Volume
		}
		if !(gain >= 0 && gain <= 1) {
			return fmt.Errorf("invalid audio gain")
		}
		index := len(labels)
		args = append(args, "-protocol_whitelist", "file,pipe", "-i", path)
		chain := fmt.Sprintf("[%d:a]atrim=start=%g:duration=%g,asetpts=PTS-STARTPTS,volume=%g", index, clip.Trim, clip.Duration, gain)
		if clip.FadeIn > 0 {
			chain += fmt.Sprintf(",afade=t=in:st=0:d=%g", clip.FadeIn)
		}
		if clip.FadeOut > 0 {
			chain += fmt.Sprintf(",afade=t=out:st=%g:d=%g", clip.Duration-clip.FadeOut, clip.FadeOut)
		}
		label := fmt.Sprintf("a%d", index)
		chain += fmt.Sprintf(",adelay=%d:all=1[%s]", int(clip.Start*1000), label)
		filters = append(filters, chain)
		labels = append(labels, "["+label+"]")
	}
	if len(labels) == 0 {
		return fmt.Errorf("the current edit has no audible source; add dialogue or narration before transcription")
	}
	filters = append(filters, fmt.Sprintf("%samix=inputs=%d:normalize=0,apad=whole_dur=%g[out]", strings.Join(labels, ""), len(labels), m.Duration))
	args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[out]", "-t", fmt.Sprintf("%g", m.Duration), "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", destination)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if e := exec.CommandContext(ctx, "ffmpeg", args...).Run(); e != nil {
		return fmt.Errorf("could not prepare the edited audio mix: %w", e)
	}
	return nil
}
