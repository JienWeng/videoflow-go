package api

import "testing"

func TestSpeechSimilarityTreatsChineseYearFormsAlike(t *testing.T) {
	if got := speechSimilarity("1977年，我们开始了。", "一九七七年我们开始了"); got < 0.99 {
		t.Fatal(got)
	}
}
