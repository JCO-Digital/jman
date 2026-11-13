package domainlookup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const (
	// RefreshAfter is how old a successful lookup gets before it is redone.
	RefreshAfter = 24 * time.Hour
	// RetryAfter is how soon a failed lookup is tried again.
	RetryAfter = 6 * time.Hour
	// ManualMinInterval is how soon after a lookup a manual one is skipped.
	ManualMinInterval = time.Minute
	// batchSize caps how many domains one RefreshDue looks up. At the
	// client's pacing that is a few minutes per slow tick, and 200 domains
	// are all refreshed within a few ticks of start-up.
	batchSize = 40
)

// DefaultClient is the client the stored lookups use; tests replace it.
var DefaultClient = NewClient()

// refreshMu keeps two RefreshDue runs from overlapping.
var refreshMu sync.Mutex

// LookupItem looks one domain item up now and stores the result. A failed
// lookup is stored with its error, keeping the last successful data.
func LookupItem(ctx context.Context, itemID, domain string) (*models.InventoryDomainLookup, error) {
	prev, err := db.GetInventoryDomainLookup(itemID)
	if err != nil {
		return nil, err
	}
	result, lookupErr := DefaultClient.Lookup(ctx, domain)
	if ctx.Err() != nil {
		// Cancelled (shutdown, client gone): not the registry's fault.
		return prev, ctx.Err()
	}

	now := time.Now().UTC()
	l := &models.InventoryDomainLookup{ItemID: itemID, Domain: domain, CheckedAt: now, Nameservers: []string{}}
	if lookupErr != nil {
		l.Error = lookupErr.Error()
		// Keep what was last found for this name, unless the registry now
		// says the domain doesn't exist.
		if prev != nil && prev.Domain == domain && !errors.Is(lookupErr, ErrNotFound) {
			l.Nameservers = prev.Nameservers
			l.Registrar = prev.Registrar
			l.ExpiresAt = prev.ExpiresAt
			l.Source = prev.Source
			l.LastSuccessAt = prev.LastSuccessAt
		}
	} else {
		l.Nameservers = result.Nameservers
		l.Registrar = result.Registrar
		l.ExpiresAt = result.ExpiresAt
		l.Source = result.Source
		l.LastSuccessAt = &now
	}
	if err := db.SaveInventoryDomainLookup(l); err != nil {
		return nil, err
	}
	return l, nil
}

// RefreshDue looks up a batch of the domain items whose lookup is due (see
// db.ListDueInventoryDomainLookups) and returns how many it looked up. It
// does nothing when another run is in progress.
func RefreshDue(ctx context.Context) (int, error) {
	if !refreshMu.TryLock() {
		return 0, nil
	}
	defer refreshMu.Unlock()

	now := time.Now()
	due, err := db.ListDueInventoryDomainLookups(now.Add(-RefreshAfter), now.Add(-RetryAfter), batchSize)
	if err != nil {
		return 0, err
	}
	failed := 0
	for i, d := range due {
		l, err := LookupItem(ctx, d.ItemID, d.Domain)
		if err != nil {
			return i, fmt.Errorf("domain lookup of %s: %w", d.Domain, err)
		}
		if l.Error != "" {
			failed++
		}
	}
	if failed > 0 {
		log.Printf("Domain lookups: %d of %d failed (stored with the item)", failed, len(due))
	}
	return len(due), nil
}
