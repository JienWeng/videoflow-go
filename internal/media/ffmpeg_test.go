package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAudioExtraction(t *testing.T) {
	m := NewMediaEngine()
	if !m.Available() {
		t.Skip("FFmpeg not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	cmd := exec.Command(m.ffmpegPath, "-y", "-f", "lavfi", "-i", "color=c=blue:s=320x240:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-c:a", "aac", "-shortest", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture %v %s", err, out)
	}
	audio := filepath.Join(dir, "audio.wav")
	if err := m.ExtractAudio(context.Background(), video, audio); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(audio); err != nil || len(data) < 44 {
		t.Fatal("missing WAV", err)
	}
}
