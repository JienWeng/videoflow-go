package composition

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Runtime is opt-in. Never invokes npx, installs packages, or accepts raw argv.
func Runtime() (string, error) {
	for _, tool := range []string{"node", "ffmpeg", "ffprobe"} {
		if _, e := exec.LookPath(tool); e != nil {
			return "", fmt.Errorf("optional HyperFrames runtime missing %s", tool)
		}
	}
	nodeCtx, nodeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer nodeCancel()
	nodeVersion, nodeErr := exec.CommandContext(nodeCtx, "node", "--version").Output()
	major, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(strings.TrimSpace(string(nodeVersion)), "v"), ".")[0])
	if nodeErr != nil || major < 22 {
		return "", fmt.Errorf("HyperFrames requires Node 22+")
	}
	browser := os.Getenv("HYPERFRAMES_BROWSER_PATH")
	if browser == "" {
		return "", fmt.Errorf("set HYPERFRAMES_BROWSER_PATH to installed Chrome; automatic downloads are disabled")
	}
	if _, e := os.Stat(browser); e != nil {
		return "", fmt.Errorf("configured Chrome unavailable")
	}
	bin := os.Getenv("VIDEOFLOW_HYPERFRAMES_BIN")
	if bin == "" {
		return "", fmt.Errorf("optional HyperFrames runtime missing: install hyperframes@%s with Node 22+, Chrome and FFmpeg, then set VIDEOFLOW_HYPERFRAMES_BIN", RuntimeVersion)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if e != nil || strings.TrimSpace(string(out)) != RuntimeVersion {
		return "", fmt.Errorf("HyperFrames runtime must be pinned to %s", RuntimeVersion)
	}
	return bin, nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 65536 {
		remaining := 65536 - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}
func Run(ctx context.Context, dir, operation string) (string, error) {
	var args []string
	switch operation {
	case "lint", "check":
		args = []string{operation, "."}
	case "snapshot":
		args = []string{"snapshot", ".", "--frames", "3", "--output", "snapshots"}
	case "render":
		args = []string{"render", ".", "--output", "render.mp4", "--workers", "1"}
	default:
		return "", fmt.Errorf("unsupported local operation")
	}
	bin, e := Runtime()
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	containProcess(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DO_NOT_TRACK=1", "HYPERFRAMES_NO_TELEMETRY=1")
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	e = cmd.Run()
	if e != nil {
		return out.String(), fmt.Errorf("local %s failed: %w", operation, e)
	}
	return out.String(), nil
}
