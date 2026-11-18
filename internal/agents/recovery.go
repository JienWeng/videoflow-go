package agents

import _ "embed"

// StudioRecovery is also installed verbatim in the two native session skill directories.
// Tool-disabled chat planners receive the same text in their system prompt.
//
//go:embed skills/studio-recovery/SKILL.md
var StudioRecovery string
