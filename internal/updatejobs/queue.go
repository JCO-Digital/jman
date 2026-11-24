package updatejobs

import (
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// How the queue picks the next job (docs/update-queue.md):
//
//   - A site runs one job at a time, and its jobs run in the order they were
//     queued: a job never overtakes an older job on the same site.
//   - Sites are ranked by the highest priority among their queued jobs, so a
//     high priority job pulls its site's older jobs forward with it. Equal
//     ranks run in queue order.
//   - Low priority jobs age: they count as normal once they have waited
//     agingStep.
//   - At most PerServer jobs run on one server at a time. Express workers
//     may go one over, so a full server doesn't hold a high job back.
//   - ExpressWorkers only take sites with a high or urgent job, so those
//     don't wait for a free general worker behind a bulk run.
//   - Plugin update jobs at the front of a site's queue run together in one
//     WP-CLI call (see takeBatch).
const (
	// Workers is how many general workers run jobs (on different sites).
	Workers = 4
	// ExpressWorkers is how many more workers only run sites that have a
	// high or urgent job queued.
	ExpressWorkers = 1
	// PerServer is how many jobs run at once on one server.
	PerServer = 2
	// MaxPluginsPerRun bounds the plugins of one `wp plugin update` call,
	// for a single job and for jobs combined into one run.
	MaxPluginsPerRun = 50
	// agingStep is how long a low priority job waits before it counts as
	// normal.
	agingStep = 30 * time.Minute
)

// ErrJobNotQueued is returned by SetPriority when the job isn't waiting in
// the queue (it doesn't exist, has started or has finished).
var ErrJobNotQueued = errors.New("the job is not queued")

// siteServers maps site IDs to the server each site runs on, for the
// per-server limit. A site missing from it has no server limit.
var siteServers = func() map[string]string {
	sites, err := cache.GetFastSiteList()
	if err != nil {
		return nil
	}
	servers := make(map[string]string, len(sites))
	for _, s := range sites {
		servers[s.ID] = serverKey(s)
	}
	return servers
}

// serverKey identifies a site's server: its provider ID, or its name for
// sites without one.
func serverKey(s models.CliSite) string {
	if s.ServerID != "" {
		return s.Provider + ":" + s.ServerID
	}
	return s.ServerName
}

// entry is a queued job and the server its site runs on.
type entry struct {
	job    models.UpdateJob
	server string
}

// claim is what a worker takes from the queue: one job, or plugin update
// jobs on one site that run together.
type claim struct {
	jobs   []models.UpdateJob
	site   string
	server string
}

// queue holds the jobs waiting to run, in the order they were queued, and
// the sites and servers with jobs in progress.
type queue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []entry
	busy    map[string]bool
	servers map[string]int
	closed  bool
	now     func() time.Time
}

func newQueue() *queue {
	qu := &queue{busy: map[string]bool{}, servers: map[string]int{}, now: time.Now}
	qu.cond = sync.NewCond(&qu.mu)
	return qu
}

func (qu *queue) push(jobs ...models.UpdateJob) {
	if len(jobs) == 0 {
		return
	}
	servers := siteServers()
	qu.mu.Lock()
	for _, job := range jobs {
		qu.pending = append(qu.pending, entry{job: job, server: servers[job.SiteID]})
	}
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

// effective is a job's priority for ordering: a low priority job counts as
// normal once it has waited agingStep.
func effective(job models.UpdateJob, now time.Time) models.UpdateJobPriority {
	if job.Priority < models.UpdateJobPriorityNormal && !job.CreatedAt.IsZero() && now.Sub(job.CreatedAt) >= agingStep {
		return models.UpdateJobPriorityNormal
	}
	return job.Priority
}

// siteRanks returns each site's rank: the highest effective priority among
// its queued jobs.
func (qu *queue) siteRanks(now time.Time) map[string]models.UpdateJobPriority {
	ranks := map[string]models.UpdateJobPriority{}
	for _, e := range qu.pending {
		p := effective(e.job, now)
		if r, ok := ranks[e.job.SiteID]; !ok || p > r {
			ranks[e.job.SiteID] = p
		}
	}
	return ranks
}

// pick returns the index in pending of the job to run next, or -1 if none
// can run now. Only the first queued job of each site is a candidate.
func (qu *queue) pick(express bool) int {
	ranks := qu.siteRanks(qu.now())
	limit := PerServer
	if express {
		limit++
	}
	best := -1
	seen := map[string]bool{}
	for i, e := range qu.pending {
		site := e.job.SiteID
		if seen[site] {
			continue
		}
		seen[site] = true
		if qu.busy[site] || (e.server != "" && qu.servers[e.server] >= limit) {
			continue
		}
		if express && ranks[site] < models.UpdateJobPriorityHigh {
			continue
		}
		// Ties keep queue order: the earlier site wins.
		if best == -1 || ranks[site] > ranks[qu.pending[best].job.SiteID] {
			best = i
		}
	}
	return best
}

// next blocks until a job can run, removes it (and the jobs that run with
// it) from the queue, marks its site and server busy and returns them.
// Express workers only take sites with a high or urgent job. ok is false
// once the queue is closed.
func (qu *queue) next(express bool) (c claim, ok bool) {
	qu.mu.Lock()
	defer qu.mu.Unlock()
	for {
		if qu.closed {
			return c, false
		}
		if i := qu.pick(express); i >= 0 {
			c = qu.takeBatch(i)
			qu.busy[c.site] = true
			if c.server != "" {
				qu.servers[c.server]++
			}
			return c, true
		}
		qu.cond.Wait()
	}
}

// batchable reports whether a job can run together with other jobs: plugin
// update jobs can, in one WP-CLI call.
func batchable(job models.UpdateJob) bool {
	return job.Kind == models.UpdateJobKindPlugins
}

// takeBatch removes the job at index i from pending, together with the
// plugin update jobs right behind it on the same site that can share its
// WP-CLI call: same allow_major, and at most MaxPluginsPerRun plugins in
// all. It stops at the site's first job that can't join, so the site's
// jobs still run in order.
func (qu *queue) takeBatch(i int) claim {
	head := qu.pending[i]
	c := claim{jobs: []models.UpdateJob{head.job}, site: head.job.SiteID, server: head.server}
	take := []int{i}
	if batchable(head.job) {
		allowMajor := jobPluginsPayload(head.job).AllowMajor
		names := map[string]bool{}
		for _, p := range jobPlugins(head.job) {
			names[p.Name] = true
		}
		for j := i + 1; j < len(qu.pending); j++ {
			e := qu.pending[j]
			if e.job.SiteID != c.site {
				continue
			}
			if !batchable(e.job) || jobPluginsPayload(e.job).AllowMajor != allowMajor {
				break
			}
			added := 0
			for _, p := range jobPlugins(e.job) {
				if !names[p.Name] {
					added++
				}
			}
			if len(names)+added > MaxPluginsPerRun {
				break
			}
			for _, p := range jobPlugins(e.job) {
				names[p.Name] = true
			}
			c.jobs = append(c.jobs, e.job)
			take = append(take, j)
		}
	}
	for k := len(take) - 1; k >= 0; k-- {
		qu.pending = slices.Delete(qu.pending, take[k], take[k]+1)
	}
	return c
}

func (qu *queue) done(c claim) {
	qu.mu.Lock()
	delete(qu.busy, c.site)
	if c.server != "" {
		qu.servers[c.server]--
		if qu.servers[c.server] <= 0 {
			delete(qu.servers, c.server)
		}
	}
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

func (qu *queue) close() {
	qu.mu.Lock()
	qu.closed = true
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

// setPriority changes a queued job's priority, in the database and in the
// queue. It holds the queue's lock throughout, so the job can't start in
// between.
func (qu *queue) setPriority(id int64, p models.UpdateJobPriority) error {
	qu.mu.Lock()
	defer qu.mu.Unlock()
	i := slices.IndexFunc(qu.pending, func(e entry) bool { return e.job.ID == id })
	if i < 0 {
		return ErrJobNotQueued
	}
	ok, err := db.SetUpdateJobPriority(id, p)
	if err != nil {
		return err
	}
	if !ok {
		return ErrJobNotQueued
	}
	qu.pending[i].job.Priority = p
	qu.cond.Broadcast()
	return nil
}

// positions returns each queued job's approximate place in the queue (1
// runs next): sites in rank order, each site's jobs in queue order. It
// ignores busy sites and servers, which only hold a job back for a while.
func (qu *queue) positions() map[int64]int {
	qu.mu.Lock()
	defer qu.mu.Unlock()
	ranks := qu.siteRanks(qu.now())
	order := slices.Clone(qu.pending)
	sort.SliceStable(order, func(a, b int) bool {
		return ranks[order[a].job.SiteID] > ranks[order[b].job.SiteID]
	})
	pos := make(map[int64]int, len(order))
	for i, e := range order {
		pos[e.job.ID] = i + 1
	}
	return pos
}

// SetPriority changes the priority of a queued job (ErrJobNotQueued if it
// isn't waiting to run).
func SetPriority(id int64, p models.UpdateJobPriority) error {
	return q.setPriority(id, p)
}

// QueuePositions returns the approximate queue position of each queued job
// (1 runs next).
func QueuePositions() map[int64]int {
	return q.positions()
}

// WithPositions sets Position on the queued jobs in jobs.
func WithPositions(jobs []models.UpdateJob) {
	pos := QueuePositions()
	for i := range jobs {
		if jobs[i].Status == models.UpdateJobQueued {
			jobs[i].Position = pos[jobs[i].ID]
		}
	}
}
