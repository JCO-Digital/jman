package wpcli

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireHostLimitsPerServer(t *testing.T) {
	// Different SSH users on one server share its slots; another server
	// (or port) has its own.
	specs := []string{"a@limit-1.test", "b@limit-1.test", "c@limit-1.test:22", "d@limit-1.test"}

	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4*len(specs); i++ {
		wg.Add(1)
		go func(spec string) {
			defer wg.Done()
			defer AcquireHost(spec)()
			n := running.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			running.Add(-1)
		}(specs[i%len(specs)])
	}

	// Another server isn't held up by the busy one.
	done := make(chan struct{})
	go func() {
		defer AcquireHost("a@limit-2.test")()
		defer AcquireHost("a@limit-1.test:2222")()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("other servers waited for limit-1.test's slots")
	}

	wg.Wait()
	if p := peak.Load(); p != MaxPerHost {
		t.Errorf("peak runs on limit-1.test = %d, want %d", p, MaxPerHost)
	}
}
