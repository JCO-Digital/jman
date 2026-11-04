package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestLatestUpdateJobBy(t *testing.T) {
	setupTestAPIDB(t)

	if job, err := LatestUpdateJobBy("nightly"); err != nil || job != nil {
		t.Fatalf("no jobs: LatestUpdateJobBy = %+v, %v", job, err)
	}
	for _, by := range []string{"nightly", "admin", "nightly", "admin"} {
		job := models.NewUpdateJob(models.UpdateJobKindCore, "site-"+by, models.CorePayload{Target: "minor"})
		job.CreatedBy = by
		if err := CreateUpdateJob(&job); err != nil {
			t.Fatal(err)
		}
	}
	job, err := LatestUpdateJobBy("nightly")
	if err != nil || job == nil || job.ID != 3 || job.CreatedBy != "nightly" || job.CreatedAt.IsZero() {
		t.Errorf("LatestUpdateJobBy = %+v, %v; want job 3", job, err)
	}
}
