//go:build windows

package composition

import (
	"os/exec"
	"time"
)

func containProcess(cmd *exec.Cmd) { cmd.WaitDelay = 3 * time.Second }
