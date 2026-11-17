package db

import (
	"path/filepath"
	"testing"
	"videoflow-go/internal/models"
)

func TestRenderBatchRollsBackBeforeQueue(t *testing.T) {
	d, e := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	jobs := []*models.RenderJob{{ID: "same", ProjectID: &project, Status: "pending", RequestJSON: []byte(`{}`)}, {ID: "same", ProjectID: &project, Status: "pending", RequestJSON: []byte(`{}`)}}
	if e = d.CreateRenderJobs(jobs); e == nil {
		t.Fatal("invalid batch committed")
	}
	saved, e := d.ListRenderJobs(project)
	if e != nil || len(saved) != 0 {
		t.Fatal("partial batch persisted", len(saved), e)
	}
}
