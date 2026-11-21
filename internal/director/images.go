package director

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ImagesAvailable checks a local CLI capability only; it never contacts a model.
func (r *Runner) ImagesAvailable(ctx context.Context) bool {
	executable, err := r.Available("codex")
	if err != nil {
		return false
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(check, executable, "features", "list")
	cmd.Env = SafeEnvironment(os.Environ())
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "image_generation" && fields[len(fields)-1] == "true" {
			return true
		}
	}
	return false
}

func CodexImageArgs(images []string) []string {
	args := CodexArgs("")
	for i, argument := range args {
		if argument == "features.image_generation=false" {
			args[i] = "features.image_generation=true"
		}
	}
	args = args[:len(args)-1]
	args = append(args, "--ephemeral")
	for _, path := range images {
		args = append(args, "--image", path)
	}
	return append(args, "-")
}
