package presets

import "fmt"

// Storytelling is independent of rendering medium and provider configuration.
type Storytelling struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Guidance    string `json:"guidance"`
}

func StorytellingPresets() []Storytelling {
	return []Storytelling{
		{"drama", "Character drama", "Dialogue, meaningful reactions and changes in relationships.", "Build scenes around a visible character goal, tension, a meaningful reaction and a changed relationship or understanding. Preserve exact agreed dialogue. Motivate close-ups and pauses; maintain eyelines, screen direction, costumes and prop states. Avoid repetitive reaction cuts and invented plot twists."},
		{"adventure", "Adventure & discovery", "A clear goal, discoveries and a satisfying payoff.", "Establish the goal and geography, reveal discoveries through visible actions, then resolve the central question. Keep the protagonist recognizable across locations. Carry relevant props and persistent states between segments; do not add extra obstacles merely to fill time."},
		{"explainer", "Guided explainer", "A recurring guide introduces places, ideas or a process.", "Use a clear question, concrete examples and a concise takeaway. When the user supplies a recurring guide, preserve its identity and role across locations. Distinguish factual claims from fictional framing; do not invent facts. Use narration only when selected, and keep labels and captions in the editor rather than baking them into generated imagery."},
		{"comedy", "Visual comedy", "Readable setups, contrast and well-timed payoffs.", "Use a clear setup, visible contrast and an earned payoff without changing the agreed premise. Leave room for reactions and readable wide staging; do not rely on frantic cuts, extra characters or unsolicited sound effects."},
	}
}

func StorytellingDirection(id string) (string, error) {
	if id == "" || id == "custom" {
		return "", nil
	}
	for _, p := range StorytellingPresets() {
		if p.ID == id {
			return "Storytelling method: " + p.Name + ". " + p.Guidance + " Reuse suitable existing character, location and prop assets before generating missing ones. Record reference identity and purpose for each segment. Group compatible internal shots into one video segment within actual model limits; shot count is not a generation-call count. Preserve the selected visual medium, language, speech choice, total duration and scene count. This guidance does not authorize tools or paid calls.", nil
		}
	}
	return "", fmt.Errorf("unknown storytelling preset; choose a current storytelling method")
}
