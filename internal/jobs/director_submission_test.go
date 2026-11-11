package jobs

import (
	"context"
	"errors"
	"testing"
	"videoflow-go/internal/models"
)

func TestFrozenRenderRequiresSubmissionValidator(t *testing.T) {
	pool := &WorkerPool{}
	job := &models.RenderJob{RequestJSON: []byte(`{"director_context":{"project_id":"p"},"voice_policy":{"mode":"saved"}}`)}
	if err := pool.validateSubmission(context.Background(), job); err == nil {
		t.Fatal("frozen request can submit without a source validator")
	}
	sentinel := errors.New("changed voice profile")
	pool.SetSubmissionValidator(func(context.Context, *models.RenderJob) error { return sentinel })
	if err := pool.validateSubmission(context.Background(), job); !errors.Is(err, sentinel) {
		t.Fatal("source validator ignored", err)
	}
}
