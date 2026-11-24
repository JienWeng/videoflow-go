package composition

import "os/exec"

// ContainProcess prepares a command so that cancelling it stops everything it started
// (its whole process group on Unix), the same limit local renders run under.
func ContainProcess(cmd *exec.Cmd) { containProcess(cmd) }
