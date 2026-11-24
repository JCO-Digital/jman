package updatejobs

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// newTestQueue returns a queue whose sites all run on servers, if given, as
// site ID → server.
func newTestQueue(t *testing.T, servers map[string]string) *queue {
	t.Helper()
	old := siteServers
	siteServers = func() map[string]string { return servers }
	t.Cleanup(func() { siteServers = old })
	qu := newQueue()
	t.Cleanup(qu.close)
	return qu
}

// qjob is a job for queue tests; kind "" is a job that never runs together
// with others.
func qjob(id int64, site string, p models.UpdateJobPriority, kind string, plugins ...string) models.UpdateJob {
	reqs := make([]models.PluginUpdateRequest, len(plugins))
	for i, name := range plugins {
		reqs[i] = models.PluginUpdateRequest{Name: name}
	}
	job := models.NewUpdateJob(kind, site, models.PluginsPayload{Plugins: reqs})
	job.ID = id
	job.Priority = p
	job.CreatedAt = time.Now()
	return job
}

func claimIDs(c claim) []int64 {
	ids := make([]int64, len(c.jobs))
	for i, j := range c.jobs {
		ids[i] = j.ID
	}
	return ids
}

// mustNext returns the next claim, failing if none is available right away.
func mustNext(t *testing.T, qu *queue, express bool) claim {
	t.Helper()
	got := make(chan claim, 1)
	go func() {
		c, ok := qu.next(express)
		if ok {
			got <- c
		}
	}()
	select {
	case c := <-got:
		return c
	case <-time.After(time.Second):
		qu.close()
		t.Fatal("no job available")
		return claim{}
	}
}

// assertBlocked checks that next has nothing to hand out right now; it then
// closes the queue to end the waiting call.
func assertBlocked(t *testing.T, qu *queue, express bool) {
	t.Helper()
	got := make(chan claim, 1)
	go func() {
		if c, ok := qu.next(express); ok {
			got <- c
		}
	}()
	select {
	case c := <-got:
		t.Errorf("got jobs %v, want none to be available", claimIDs(c))
	case <-time.After(100 * time.Millisecond):
	}
	qu.close()
}

func TestQueueRunsOneJobPerSiteAtATime(t *testing.T) {
	qu := newTestQueue(t, nil)
	qu.push(qjob(1, "s1", 0, ""), qjob(2, "s1", 0, ""), qjob(3, "s2", 0, ""))

	first := mustNext(t, qu, false)
	second := mustNext(t, qu, false)
	if first.jobs[0].ID != 1 || second.jobs[0].ID != 3 {
		t.Fatalf("got jobs %v, %v; want 1 then 3 (job 2 waits for site s1)", claimIDs(first), claimIDs(second))
	}

	got := make(chan int64, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if c, ok := qu.next(false); ok {
			got <- c.jobs[0].ID
		}
	}()
	select {
	case id := <-got:
		t.Fatalf("job %d started while site s1 was busy", id)
	case <-time.After(100 * time.Millisecond):
	}
	qu.done(first)
	select {
	case id := <-got:
		if id != 2 {
			t.Errorf("got job %d, want 2", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job 2 did not start after site s1 was released")
	}
	wg.Wait()
}

func TestQueueRunsHigherPriorityFirst(t *testing.T) {
	qu := newTestQueue(t, nil)
	qu.push(
		qjob(1, "s1", models.UpdateJobPriorityLow, ""),
		qjob(2, "s2", models.UpdateJobPriorityNormal, ""),
		qjob(3, "s3", models.UpdateJobPriorityNormal, ""),
		qjob(4, "s4", models.UpdateJobPriorityUrgent, ""),
		qjob(5, "s5", models.UpdateJobPriorityHigh, ""),
	)
	var order []int64
	for range 5 {
		order = append(order, mustNext(t, qu, false).jobs[0].ID)
	}
	if want := []int64{4, 5, 2, 3, 1}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v (by priority, then queue order)", order, want)
	}
}

func TestQueueKeepsSiteOrderAndPullsSiteForward(t *testing.T) {
	qu := newTestQueue(t, nil)
	qu.push(
		qjob(1, "s2", models.UpdateJobPriorityNormal, ""),
		qjob(2, "s1", models.UpdateJobPriorityNormal, ""),
		qjob(3, "s1", models.UpdateJobPriorityHigh, ""),
	)
	first := mustNext(t, qu, false)
	if id := first.jobs[0].ID; id != 2 {
		t.Fatalf("first job = %d, want 2: site s1's high job pulls the site forward, but its older job runs first", id)
	}
	if id := mustNext(t, qu, false).jobs[0].ID; id != 1 {
		t.Errorf("second job = %d, want 1 (site s1 is busy)", id)
	}
	qu.done(first)
	if id := mustNext(t, qu, false).jobs[0].ID; id != 3 {
		t.Errorf("third job = %d, want 3", id)
	}
}

func TestQueueAgesLowPriorityJobs(t *testing.T) {
	qu := newTestQueue(t, nil)
	old := qjob(1, "s1", models.UpdateJobPriorityLow, "")
	old.CreatedAt = time.Now().Add(-agingStep - time.Minute)
	recent := qjob(2, "s2", models.UpdateJobPriorityLow, "")
	qu.push(recent, qjob(3, "s3", models.UpdateJobPriorityNormal, ""), old)

	var order []int64
	for range 3 {
		order = append(order, mustNext(t, qu, false).jobs[0].ID)
	}
	// The old low job counts as normal; job 3 was queued before it.
	if want := []int64{3, 1, 2}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestExpressWorkerOnlyTakesHighPrioritySites(t *testing.T) {
	qu := newTestQueue(t, nil)
	qu.push(
		qjob(1, "s1", models.UpdateJobPriorityNormal, ""),
		qjob(2, "s2", models.UpdateJobPriorityNormal, ""),
		qjob(3, "s2", models.UpdateJobPriorityHigh, ""),
	)
	// Site s2 has a high job, so the express worker takes its first job.
	if id := mustNext(t, qu, true).jobs[0].ID; id != 2 {
		t.Fatalf("express worker took job %d, want 2", id)
	}
	// Job 3 waits for site s2; job 1 is only normal.
	assertBlocked(t, qu, true)
}

func TestQueueLimitsJobsPerServer(t *testing.T) {
	qu := newTestQueue(t, map[string]string{"s1": "srv-a", "s2": "srv-a", "s3": "srv-a", "s4": "srv-b"})
	qu.push(
		qjob(1, "s1", 0, ""),
		qjob(2, "s2", 0, ""),
		qjob(3, "s3", models.UpdateJobPriorityUrgent, ""),
		qjob(4, "s4", 0, ""),
	)
	var ids []int64
	var claims []claim
	for range 3 {
		c := mustNext(t, qu, false)
		claims = append(claims, c)
		ids = append(ids, c.jobs[0].ID)
	}
	// srv-a is full after jobs 3 and 1, so job 2 waits for one of them.
	if want := []int64{3, 1, 4}; !slices.Equal(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	qu.done(claims[0])
	if id := mustNext(t, qu, false).jobs[0].ID; id != 2 {
		t.Errorf("after a srv-a job finished, got job %d, want 2", id)
	}
}

func TestExpressWorkerGoesOneOverTheServerLimit(t *testing.T) {
	qu := newTestQueue(t, map[string]string{"s1": "srv-a", "s2": "srv-a", "s3": "srv-a", "s4": "srv-a"})
	qu.push(qjob(1, "s1", 0, ""), qjob(2, "s2", 0, ""))
	// Bulk jobs fill srv-a.
	mustNext(t, qu, false)
	mustNext(t, qu, false)
	qu.push(qjob(3, "s3", models.UpdateJobPriorityHigh, ""))
	if id := mustNext(t, qu, true).jobs[0].ID; id != 3 {
		t.Fatalf("express worker took job %d, want 3 on the full server", id)
	}
	// Now srv-a runs PerServer+1 jobs: not even the express worker adds one.
	qu.push(qjob(4, "s4", models.UpdateJobPriorityHigh, ""))
	assertBlocked(t, qu, true)
}

func TestQueueRunsPluginUpdatesOnASiteTogether(t *testing.T) {
	qu := newTestQueue(t, nil)
	plugins := models.UpdateJobKindPlugins
	qu.push(
		qjob(1, "s1", 0, plugins, "a", "b"),
		qjob(2, "s2", 0, plugins, "x"),
		qjob(3, "s1", models.UpdateJobPriorityHigh, plugins, "b", "c"),
		qjob(4, "s1", 0, models.UpdateJobKindActivate, "c"),
		qjob(5, "s1", 0, plugins, "d"),
	)
	first := mustNext(t, qu, false)
	if got := claimIDs(first); !slices.Equal(got, []int64{1, 3}) {
		t.Fatalf("first run = %v, want jobs 1 and 3 together (job 4 isn't an update and job 5 is behind it)", got)
	}
	if got := claimIDs(mustNext(t, qu, false)); !slices.Equal(got, []int64{2}) {
		t.Errorf("second run = %v, want [2]", got)
	}
	qu.done(first)
	c := mustNext(t, qu, false)
	if got := claimIDs(c); !slices.Equal(got, []int64{4}) {
		t.Errorf("third run = %v, want [4]", got)
	}
	qu.done(c)
	if got := claimIDs(mustNext(t, qu, false)); !slices.Equal(got, []int64{5}) {
		t.Errorf("fourth run = %v, want [5]", got)
	}
}

func TestQueueBatchLimits(t *testing.T) {
	t.Run("allow_major", func(t *testing.T) {
		qu := newTestQueue(t, nil)
		major := qjob(2, "s1", 0, models.UpdateJobKindPlugins, "b")
		major.Payload = models.EncodePayload(models.PluginsPayload{Plugins: []models.PluginUpdateRequest{{Name: "b"}}, AllowMajor: true})
		qu.push(qjob(1, "s1", 0, models.UpdateJobKindPlugins, "a"), major)
		if got := claimIDs(mustNext(t, qu, false)); !slices.Equal(got, []int64{1}) {
			t.Errorf("run = %v, want job 1 alone: job 2 allows major updates", got)
		}
	})
	t.Run("plugin count", func(t *testing.T) {
		qu := newTestQueue(t, nil)
		var many []string
		for i := range MaxPluginsPerRun {
			many = append(many, fmt.Sprintf("p%d", i))
		}
		qu.push(
			qjob(1, "s1", 0, models.UpdateJobKindPlugins, many[:MaxPluginsPerRun-1]...),
			// Already in job 1, so it adds nothing.
			qjob(2, "s1", 0, models.UpdateJobKindPlugins, "p0"),
			qjob(3, "s1", 0, models.UpdateJobKindPlugins, "new-1", "new-2"),
		)
		if got := claimIDs(mustNext(t, qu, false)); !slices.Equal(got, []int64{1, 2}) {
			t.Errorf("run = %v, want jobs 1 and 2: job 3 would go past %d plugins", got, MaxPluginsPerRun)
		}
	})
}

func TestQueuePositions(t *testing.T) {
	qu := newTestQueue(t, nil)
	qu.push(
		qjob(1, "s1", 0, ""),
		qjob(2, "s2", 0, ""),
		qjob(3, "s1", 0, ""),
		qjob(4, "s3", models.UpdateJobPriorityHigh, ""),
	)
	got := qu.positions()
	want := map[int64]int{4: 1, 1: 2, 2: 3, 3: 4}
	for id, pos := range want {
		if got[id] != pos {
			t.Errorf("position of job %d = %d, want %d (all: %v)", id, got[id], pos, got)
		}
	}
}

func TestSetPriority(t *testing.T) {
	setupRunnerTest(t)
	jobs := []models.UpdateJob{
		pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a"}),
		pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "b"}),
	}
	jobs[1].SiteID = "22222222-2222-2222-2222-222222222222"
	if _, err := EnqueueAll(jobs); err != nil {
		t.Fatal(err)
	}
	if err := SetPriority(jobs[1].ID, models.UpdateJobPriorityUrgent); err != nil {
		t.Fatal(err)
	}
	if stored := storedJob(t, jobs[1].ID); stored.Priority != models.UpdateJobPriorityUrgent {
		t.Errorf("stored priority = %v, want urgent", stored.Priority)
	}
	if pos := QueuePositions(); pos[jobs[1].ID] != 1 {
		t.Errorf("positions = %v, want job %d first", pos, jobs[1].ID)
	}
	c := mustNext(t, q, false)
	if c.jobs[0].ID != jobs[1].ID {
		t.Errorf("next job = %d, want the urgent job %d", c.jobs[0].ID, jobs[1].ID)
	}
	if err := SetPriority(jobs[1].ID, models.UpdateJobPriorityHigh); !errors.Is(err, ErrJobNotQueued) {
		t.Errorf("SetPriority on a started job = %v, want ErrJobNotQueued", err)
	}
}

func TestRunBatchSharesOneCallAndKeepsResultsPerJob(t *testing.T) {
	setupRunnerTest(t)
	var calls [][]string
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		calls = append(calls, plugins)
		return []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: "Updated"},
			{Name: "c", OldVersion: "3.0", NewVersion: "3.1", Status: "Updated"},
		}, nil
	}
	syncs := 0
	syncSiteExposures = func(string) error { syncs++; return nil }

	first := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}, models.PluginUpdateRequest{Name: "b", OldVersion: "2.0"})
	first.CreatedBy = "alice"
	second := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "b", OldVersion: "2.0"}, models.PluginUpdateRequest{Name: "c", OldVersion: "3.0"})
	second.CreatedBy = "bob"
	for _, j := range []*models.UpdateJob{&first, &second} {
		if err := db.CreateUpdateJob(j); err != nil {
			t.Fatal(err)
		}
	}
	runBatch([]models.UpdateJob{first, second})

	if len(calls) != 1 || !slices.Equal(calls[0], []string{"a", "b", "c"}) {
		t.Errorf("WP-CLI calls = %v, want one call for a, b and c", calls)
	}
	if syncs != 1 {
		t.Errorf("exposure syncs = %d, want 1", syncs)
	}
	for _, tc := range []struct {
		job  models.UpdateJob
		want []string
	}{{first, []string{"a", "b"}}, {second, []string{"b", "c"}}} {
		stored := storedJob(t, tc.job.ID)
		if stored.Status != models.UpdateJobDone || stored.BatchID != first.ID {
			t.Errorf("job %d: status %q, batch %d; want done in batch %d", stored.ID, stored.Status, stored.BatchID, first.ID)
		}
		var names []string
		for _, r := range stored.Results {
			if r.Status != models.UpdateUpdated {
				t.Errorf("job %d: result %+v, want updated", stored.ID, r)
			}
			names = append(names, r.Name)
		}
		if !slices.Equal(names, tc.want) {
			t.Errorf("job %d results = %v, want %v", stored.ID, names, tc.want)
		}
		if stored.TextDiff == nil || stored.TextDiff.Status != models.TextDiffDone {
			t.Errorf("job %d text diff = %+v, want a finished one", stored.ID, stored.TextDiff)
		}
	}

	ledger, err := db.GetSiteUpdateLedger(testSite)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]bool{}
	for _, e := range ledger {
		by[e.UpdatedBy] = true
	}
	if len(ledger) != 2 || !by["alice"] || !by["bob"] {
		t.Errorf("ledger = %+v, want one entry by alice and one by bob", ledger)
	}
}

func TestRunBatchFailsEveryJobWhenSiteIsGone(t *testing.T) {
	setupRunnerTest(t)
	findSite = func(string) (*models.CliSite, error) { return nil, errors.New("site gone") }
	a := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a"})
	b := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "b"})
	for _, j := range []*models.UpdateJob{&a, &b} {
		if err := db.CreateUpdateJob(j); err != nil {
			t.Fatal(err)
		}
	}
	runBatch([]models.UpdateJob{a, b})
	for _, j := range []models.UpdateJob{a, b} {
		stored := storedJob(t, j.ID)
		if stored.Status != models.UpdateJobFailed || stored.Error != "site gone" {
			t.Errorf("job %d: status %q error %q, want failed with the site error", j.ID, stored.Status, stored.Error)
		}
	}
}
