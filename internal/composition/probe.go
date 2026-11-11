package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Probe reads local source metadata without decoding, transcription or providers.
func Probe(ctx context.Context, path string) (duration float64, width, height int, audio bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration:stream=codec_type,width,height", "-of", "json", path).Output()
	if e != nil {
		err = fmt.Errorf("local ffprobe required to inspect source: %w", e)
		return
	}
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if e = json.Unmarshal(out, &p); e != nil {
		err = e
		return
	}
	duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	for _, s := range p.Streams {
		if s.Type == "audio" {
			audio = true
		}
		if s.Type == "video" {
			width = s.Width
			height = s.Height
		}
	}
	return
}
