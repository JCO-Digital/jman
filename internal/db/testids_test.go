package db

import "fmt"

// testSiteID returns a valid, deterministic site UUID for tests. UUIDs built
// from increasing n sort in the same order as n.
func testSiteID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}
