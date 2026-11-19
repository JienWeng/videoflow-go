package costs

import "testing"

func TestVideoUsesSeedanceTokenPrice(t *testing.T) {
	// fal lists the fast model at $0.2419 per second of 720p video.
	got, basis := Video("fal", "bytedance/seedance-2.0/fast/image-to-video", "720p", "16:9", 10, nil)
	if basis != Rate || got < 2.41 || got > 2.43 {
		t.Fatalf("10 s of 720p = %v (%s), want about 2.42", got, basis)
	}
	tall, _ := Video("fal", "bytedance/seedance-2.0/fast/image-to-video", "", "9:16", 10, nil)
	if tall != got {
		t.Fatalf("tall 720p %v should cost the same as wide %v", tall, got)
	}
	small, _ := Video("fal", "bytedance/seedance-2.0/fast/image-to-video", "480p", "9:16", 4, nil)
	if small <= 0 || small >= got/4 {
		t.Fatalf("4 s of 480p = %v, want well under a quarter of %v", small, got)
	}
	live, _ := Video("fal", "bytedance/seedance-2.0/fast/image-to-video", "720p", "16:9", 10, &LivePrice{UnitPrice: 0.02, Unit: "1k tokens"})
	if live < 4.3 || live > 4.4 {
		t.Fatalf("live token price not applied: %v", live)
	}
	if _, basis := Video("fal", "some/new-model", "720p", "16:9", 5, nil); basis != Unpriced {
		t.Fatalf("unknown model should be unpriced, got %s", basis)
	}
	if amount, _ := Video("fal", "some/new-model", "720p", "16:9", 5, &LivePrice{UnitPrice: 0.1, Unit: "second"}); amount != 0.5 {
		t.Fatalf("per-second live price = %v, want 0.5", amount)
	}
}

func TestImageAndVoice(t *testing.T) {
	usage := &ImageUsage{InputTokens: 300, OutputTokens: 5000}
	usage.Details.Text, usage.Details.Image = 100, 200
	got, basis := Image("openai", "gpt-image-2", usage)
	if basis != Metered || got != 0.1521 {
		t.Fatalf("metered picture = %v (%s), want 0.1521", got, basis)
	}
	if _, basis := Image("openai", "gpt-image-2", nil); basis != Estimate {
		t.Fatalf("picture without usage should be an estimate, got %s", basis)
	}
	if amount, basis := Image("codex", "native-image-generation", nil); amount != 0 || basis != Included {
		t.Fatalf("Codex pictures are part of the plan: %v %s", amount, basis)
	}
	if amount, _ := Voice("fish", "s2.1-pro", string(make([]byte, 1000))); amount != 0.015 {
		t.Fatalf("1,000 bytes of speech = %v, want 0.015", amount)
	}
	if amount, basis := Voice("fish", "s2.1-pro-free", "hello"); amount != 0 || basis != Included {
		t.Fatalf("free voice model should cost nothing: %v %s", amount, basis)
	}
}
