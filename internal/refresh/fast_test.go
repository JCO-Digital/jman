package refresh

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubFastRefresh replaces the SpinupWP fetch with fn and resets the
// refresh state for the test.
func stubFastRefresh(t *testing.T, fn func() (int, int, error)) {
	t.Helper()
	old := refreshServersAndSites
	refreshServersAndSites = fn
	fast.last = FastResult{}
	t.Cleanup(func() {
		refreshServersAndSites = old
		fast.last = FastResult{}
	})
}

func TestRefreshServersAndSitesSharesInFlightFetch(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	stubFastRefresh(t, func() (int, int, error) {
		calls.Add(1)
		<-release
		return 2, 5, nil
	})

	var wg sync.WaitGroup
	results := make([]FastResult, 3)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = RefreshServersAndSites()
		}(i)
	}
	// Let every caller reach the in-flight fetch before it finishes.
	for {
		fast.mu.Lock()
		started := fast.inflight != nil
		fast.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Fatalf("fetch ran %d times, want 1", n)
	}
	for i, r := range results {
		if r.Servers != 2 || r.Sites != 5 || r.RefreshedAt.IsZero() {
			t.Errorf("result %d = %+v", i, r)
		}
	}
}

func TestRequestRefreshSkipsRecentRefresh(t *testing.T) {
	var calls atomic.Int32
	stubFastRefresh(t, func() (int, int, error) {
		calls.Add(1)
		return 1, 1, nil
	})

	if r, err := RequestRefresh(); err != nil || r.Skipped {
		t.Fatalf("first request = %+v, %v; want a refresh", r, err)
	}
	r, err := RequestRefresh()
	if err != nil || !r.Skipped || r.RefreshedAt.IsZero() {
		t.Fatalf("second request = %+v, %v; want skipped with the last refresh time", r, err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("fetch ran %d times, want 1", n)
	}

	// Once the interval has passed, a request refreshes again.
	fast.mu.Lock()
	fast.last.RefreshedAt = time.Now().Add(-2 * ManualRefreshMinInterval)
	fast.mu.Unlock()
	if r, _ := RequestRefresh(); r.Skipped {
		t.Error("request after the interval was skipped")
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("fetch ran %d times, want 2", n)
	}
}

func TestRefreshServersAndSitesErrorKeepsLastSuccess(t *testing.T) {
	stubFastRefresh(t, func() (int, int, error) { return 3, 4, nil })
	ok, _ := RefreshServersAndSites()

	refreshServersAndSites = func() (int, int, error) { return 0, 0, errors.New("spinupwp down") }
	r, err := RefreshServersAndSites()
	if err == nil {
		t.Fatal("expected the fetch error")
	}
	if r.RefreshedAt != ok.RefreshedAt || LastRefresh() != ok.RefreshedAt {
		t.Errorf("failed refresh changed the last refresh time: %+v", r)
	}
}
