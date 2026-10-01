package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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

func (m *MediaEngine) MakeThumbnail(ctx context.Context, videoPath, destPath string, atSeconds float64) error {
	if !m.Available() {
		return fmt.Errorf("ffmpeg not found")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.ffmpegPath,
		"-y",
		"-ss", fmt.Sprintf("%.2f", atSeconds),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "3",
		destPath,
	)
	return cmd.Run()
}

func (m *MediaEngine) ExtractLastFrame(ctx context.Context, videoPath, destPath string) error {
	if !m.Available() {
		return fmt.Errorf("ffmpeg not found")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.ffmpegPath,
		"-y",
		"-sseof", "-3",
		"-i", videoPath,
		"-update", "1",
		"-q:v", "2",
		destPath,
	)
	return cmd.Run()
}
