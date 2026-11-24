package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// earshot (MIT, https://github.com/Leonxlnx/earshot) makes sound effects, ambience and
// music loops on the CPU from words, and with its listen extra describes what a file
// sounds like. The Docker image installs it from this commit; a native install sets
// VIDEOFLOW_EARSHOT_BIN. Nothing is installed or downloaded while a request runs.
const EarshotCommit = "7a7f7c5c70d4a763a1eca4b3666af7c77e313f97"

// Earshot is the installed earshot command, or why there is none.
func Earshot() (string, error) {
	bin := os.Getenv("VIDEOFLOW_EARSHOT_BIN")
	if bin == "" {
		found, err := exec.LookPath("earshot")
		if err != nil {
			return "", fmt.Errorf("earshot is not installed on the backend (the Docker image includes it; natively, install earshot at %s and set VIDEOFLOW_EARSHOT_BIN)", EarshotCommit[:7])
		}
		bin = found
	}
	if !filepath.IsAbs(bin) {
		return "", fmt.Errorf("VIDEOFLOW_EARSHOT_BIN must be an absolute path")
	}
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		return "", fmt.Errorf("earshot is set to %s, which is missing", bin)
	}
	return bin, nil
}

// EarshotListening says whether earshot's listening extra (CPU torch and two models
// already on disk) is installed. It is a setting, because the models are large: the
// image is built with EARSHOT_LISTEN=1, which sets VIDEOFLOW_EARSHOT_LISTEN=1.
func EarshotListening() bool { return os.Getenv("VIDEOFLOW_EARSHOT_LISTEN") == "1" }

// EarshotEnvironment keeps earshot offline (a missing model is an error, never a
// download) and small on a two-core machine.
func EarshotEnvironment() []string {
	return []string{"EARSHOT_OFFLINE=1", "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "EARSHOT_THREADS=1", "OPENBLAS_NUM_THREADS=1"}
}

// ListenLimit is how long listening through one film may take.
var ListenLimit = 10 * time.Minute

// Listen runs earshot's timeline on a rendered film: events, sections, loudness and
// sync with the picture cuts. It returns earshot's plain-language summary and its
// JSON result (at most 1 MB).
func Listen(ctx context.Context, film string) (string, json.RawMessage, error) {
	if !EarshotListening() {
		return "", nil, fmt.Errorf("listening is off: build the image with EARSHOT_LISTEN=1 (about 1.1 GB of models, fetched at build time) or set VIDEOFLOW_EARSHOT_LISTEN=1 where the listen extra and its models are installed")
	}
	bin, err := Earshot()
	if err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp("", "videoflow-listen-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, ListenLimit)
	defer cancel()
	result := filepath.Join(dir, "timeline.json")
	cmd := exec.CommandContext(ctx, bin, "timeline", film, "--video", film, "--out", result)
	containProcess(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), EarshotEnvironment()...)
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err = cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", nil, fmt.Errorf("listening stopped after %s because it took too long", ListenLimit)
		}
		return "", nil, fmt.Errorf("earshot could not listen to the film: %s", strings.TrimSpace(lastLine(out.String())))
	}
	info, err := os.Stat(result)
	if err != nil || info.Size() > 1<<20 {
		return "", nil, fmt.Errorf("earshot gave no readable result")
	}
	raw, err := os.ReadFile(result)
	if err != nil || !json.Valid(raw) {
		return "", nil, fmt.Errorf("earshot gave no readable result")
	}
	return strings.TrimSpace(out.String()), raw, nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}

// MakeLimit is how long making one sound may take.
var MakeLimit = 10 * time.Minute

// MakeSound has earshot make a sound from words ("slow sparse piano, seamless music
// loop") into out, a .wav file, seconds long. It runs offline on the CPU and is free;
// it fails when earshot is not installed.
func MakeSound(ctx context.Context, words string, seconds int, out string) error {
	bin, err := Earshot()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(strings.ToLower(out), ".wav") || seconds < 1 || seconds > 120 || strings.TrimSpace(words) == "" {
		return fmt.Errorf("a sound needs words, a .wav file and 1-120 seconds")
	}
	ctx, cancel := context.WithTimeout(ctx, MakeLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "make", words, "-o", out, "--dur", fmt.Sprint(seconds))
	containProcess(cmd)
	cmd.Dir = filepath.Dir(out)
	cmd.Env = append(os.Environ(), EarshotEnvironment()...)
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("making the sound stopped after %s because it took too long", MakeLimit)
		}
		return fmt.Errorf("earshot could not make the sound: %s", strings.TrimSpace(lastLine(output.String())))
	}
	if info, e := os.Stat(out); e != nil || info.Size() == 0 {
		return fmt.Errorf("earshot made no sound file")
	}
	return nil
}
