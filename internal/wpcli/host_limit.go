package wpcli

import "sync"

// MaxPerHost is how many WP-CLI runs a bulk read (the refresh's sweep of
// every site) keeps going at once on one server. Each run starts
// WordPress on the server, so a global limit alone could put all of them
// on the one server that hosts many of the sites.
const MaxPerHost = 3

var (
	hostSlotsMu sync.Mutex
	hostSlots   = map[string]chan struct{}{}
)

// AcquireHost waits for one of the MaxPerHost slots of the server an SSH
// spec ("user@host[:port]") points at and returns the function that frees
// it. Sites on one server share its slots whatever their SSH user. Take
// the host slot before any global limit, so that runs waiting for a busy
// server don't hold up runs on other servers.
func AcquireHost(sshSpec string) (release func()) {
	t := parseSSHSpec(sshSpec)
	key := t.knownHostsName()

	hostSlotsMu.Lock()
	slots, ok := hostSlots[key]
	if !ok {
		slots = make(chan struct{}, MaxPerHost)
		hostSlots[key] = slots
	}
	hostSlotsMu.Unlock()

	slots <- struct{}{}
	return func() { <-slots }
}
