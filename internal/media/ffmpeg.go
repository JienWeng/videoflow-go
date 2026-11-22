package media

import (
	"context"
	"fmt"
	"os/exec"
)

type MediaEngine struct {
	ffmpegPath string
}

func NewMediaEngine() *MediaEngine {
	path, _ := exec.LookPath("ffmpeg")
	return &MediaEngine{ffmpegPath: path}
}

func (m *MediaEngine) Available() bool {
	return m.ffmpegPath != ""
}

func (m *MediaEngine) ExtractAudio(ctx context.Context, videoPath, destPath string) error {
	if !m.Available() {
		return fmt.Errorf("FFmpeg is required to extract video audio")
	}
	cmd := exec.CommandContext(ctx, m.ffmpegPath, "-y", "-i", videoPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", destPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract audio: %w: %s", err, out)
	}
	return nil
}
