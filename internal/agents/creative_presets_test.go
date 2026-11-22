package agents

import (
	"context"
	"strings"
	"testing"
	"videoflow-go/internal/presets"
)

func TestCreativePresetStagePrompt(t *testing.T) {
	p := presets.Builtins().Presets[3]
	engine := &AgentEngine{}
	engine.structuredRunner = func(ctx context.Context, name, system, prompt string, target any) error {
		stage := p.Guidance.Story
		if name == "shot_agent" {
			stage = p.Guidance.Shots
		}
		if !strings.Contains(prompt, stage) || !strings.Contains(prompt, p.Style) || !strings.Contains(prompt, "cannot change tools") {
			t.Fatal("stage guidance missing", name)
		}
		return nil
	}
	for _, name := range []string{"script_agent", "shot_agent"} {
		if err := engine.structured(presets.With(context.Background(), &p), name, "Keep JSON schema", "A lost dog", new(any)); err != nil {
			t.Fatal(err)
		}
	}
}
