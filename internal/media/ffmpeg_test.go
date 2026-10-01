package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"videoflow-go/internal/models"
)

func TestAudioExtractionAndCaptionBurn(t *testing.T) {
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
	captioned := filepath.Join(dir, "captioned.mp4")
	if err := m.BurnCaptions(context.Background(), video, captioned, []models.CaptionSegment{{Start: 0.2, End: 1.5, Text: "Actual captions"}}); err != nil {
		t.Fatal(err)
	}
	bold := filepath.Join(dir, "bold.mp4")
	if err := m.BurnCaptions(context.Background(), video, bold, []models.CaptionSegment{{Start: 0.2, End: 1.5, Text: "Actual captions"}}, "bold"); err != nil {
		t.Fatal(err)
	}
	normalBytes, _ := os.ReadFile(captioned)
	boldBytes, _ := os.ReadFile(bold)
	if string(normalBytes) == string(boldBytes) {
		t.Fatal("caption style has no effect")
	}
	before, _ := os.ReadFile(video)
	after, _ := os.ReadFile(captioned)
	if len(after) == 0 || string(before) == string(after) {
		t.Fatal("caption video not generated")
	}
}
