package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

func TestGenerateShotsNormalizesNumericTiming(t *testing.T) {
	for _, tc := range []struct {
		name, durations string
		total           int
		want            []int
	}{
		{"fractional regression", `1.2,1.3,2.5`, 5, []int{1, 1, 3}},
		{"decimal integers", `2.0,3.0`, 5, []int{2, 3}},
		{"existing integers", `2,3`, 5, []int{2, 3}},
		{"sum preserved", `2,2`, 5, []int{3, 2}},
		{"positive minimum", `0.2,0.3`, 2, []int{1, 1}},
		{"reserve minimum at integer boundary", `1e308,1`, math.MaxInt, []int{math.MaxInt - 1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var values []json.RawMessage
			if err := json.Unmarshal([]byte("["+tc.durations+"]"), &values); err != nil {
				t.Fatal(err)
			}
			raw := `{"shots":[`
			for i, v := range values {
				if i > 0 {
					raw += ","
				}
				raw += fmt.Sprintf(`{"shot_order":%d,"duration":%s,"prompt":"Puppy finds mother","camera":"wide","movement":"still"}`, i+1, v)
			}
			raw += "]}"
			engine := NewAgentEngine(nil, "")
			calls := 0
			engine.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
				calls++
				return json.Unmarshal([]byte(raw), target)
			})
			shots, err := engine.GenerateShots(context.Background(), "Reunion", "Dog story", tc.total)
			if err != nil {
				t.Fatal(err)
			}
			sum := 0
			for i, shot := range shots {
				if shot.Duration != tc.want[i] || shot.Prompt != "Puppy finds mother" || shot.Order != i+1 {
					t.Fatalf("unexpected shot %#v", shot)
				}
				sum += shot.Duration
			}
			if sum != tc.total || calls != 1 {
				t.Fatalf("total=%d calls=%d", sum, calls)
			}
		})
	}
}

func TestGenerateShotsRejectsInvalidTiming(t *testing.T) {
	for _, duration := range []string{`0`, `-1`, `null`, `"1.2"`, `1e999`} {
		t.Run(duration, func(t *testing.T) {
			e := NewAgentEngine(nil, "")
			e.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
				return json.Unmarshal([]byte(fmt.Sprintf(`{"shots":[{"shot_order":1,"duration":%s,"prompt":"Dog"}]}`, duration)), target)
			})
			if _, err := e.GenerateShots(context.Background(), "Dog", "Dog", 5); err == nil {
				t.Fatal("accepted invalid timing")
			}
		})
	}
}

func TestGenerateShotsRejectsImpossibleTimeline(t *testing.T) {
	e := NewAgentEngine(nil, "")
	calls := 0
	e.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		calls++
		return json.Unmarshal([]byte(`{"shots":[{"shot_order":1,"duration":0.5,"prompt":"Dog"},{"shot_order":2,"duration":0.5,"prompt":"Mother"}]}`), target)
	})
	if _, err := e.GenerateShots(context.Background(), "Dog", "Dog", 1); err == nil {
		t.Fatal("accepted two positive integer shots within one second")
	}
	if _, err := e.GenerateShots(context.Background(), "Dog", "Dog", 0); err == nil {
		t.Fatal("accepted invalid scene duration")
	}
	if calls != 1 {
		t.Fatalf("invalid scene invoked engine: %d calls", calls)
	}
}

func TestGenerateShotsRejectsOverflowingProposedTotal(t *testing.T) {
	e := NewAgentEngine(nil, "")
	e.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		return json.Unmarshal([]byte(`{"shots":[{"shot_order":1,"duration":1e308,"prompt":"Dog"},{"shot_order":2,"duration":1e308,"prompt":"Mother"}]}`), target)
	})
	if _, err := e.GenerateShots(context.Background(), "Dog", "Dog", 5); err == nil {
		t.Fatal("accepted non-finite total")
	}
}
