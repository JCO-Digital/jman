package updatejobs

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/wpcli"
)

// stagedZipTTL is how long a staged ZIP waits for its update jobs.
const stagedZipTTL = time.Hour

// StagedZip is a plugin ZIP stored in UploadDir, checked and waiting for
// the update jobs that will install it on a set of sites.
type StagedZip struct {
	ID string `json:"id"`
	// Filename is the uploaded or downloaded file's name, or for a ZIP
	// copied from a site "<slug>.zip".
	Filename  string              `json:"filename"`
	Size      int64               `json:"size"`
	Plugin    wpcli.PluginZipInfo `json:"plugin"`
	CreatedBy string              `json:"created_by"`
	ExpiresAt time.Time           `json:"expires_at"`
	// SourceSite is the site the plugin's files were copied from, if they
	// weren't uploaded or downloaded.
	SourceSite *ZipSourceSite `json:"source_site,omitempty"`
	Path       string         `json:"-"`
}

// ZipSourceSite is the site a staged ZIP's files were copied from.
type ZipSourceSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// ScannedAt is the site's latest malware scan, which found nothing.
	ScannedAt time.Time `json:"scanned_at"`
	// Skipped lists files that weren't copied: symlinks and other files
	// that aren't regular files (at most maxSkippedListed).
	Skipped []string `json:"skipped,omitempty"`
}

var staged = struct {
	sync.Mutex
	zips map[string]StagedZip
}{zips: map[string]StagedZip{}}

// StageZip registers the ZIP at z.Path, which then belongs to the staging
// area: it's removed when it expires unless TakeStagedZip hands it to jobs.
func StageZip(z StagedZip) (StagedZip, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return StagedZip{}, err
	}
	z.ID = hex.EncodeToString(id)
	z.ExpiresAt = time.Now().Add(stagedZipTTL)

	staged.Lock()
	defer staged.Unlock()
	sweepStagedLocked(time.Now())
	staged.zips[z.ID] = z
	return z, nil
}

// TakeStagedZip removes a staged ZIP from the staging area and returns it;
// the caller now owns its file. ok is false if it doesn't exist (anymore)
// or was staged by another user.
func TakeStagedZip(id, user string) (z StagedZip, ok bool) {
	staged.Lock()
	defer staged.Unlock()
	sweepStagedLocked(time.Now())
	z, ok = staged.zips[id]
	if !ok || z.CreatedBy != user {
		return StagedZip{}, false
	}
	delete(staged.zips, id)
	return z, true
}

// RestageZip puts back a ZIP taken with TakeStagedZip when none of its jobs
// could be created, so the user can try again.
func RestageZip(z StagedZip) {
	staged.Lock()
	defer staged.Unlock()
	staged.zips[z.ID] = z
}

// SweepStagedZips removes expired staged ZIPs.
func SweepStagedZips() {
	staged.Lock()
	defer staged.Unlock()
	sweepStagedLocked(time.Now())
}

func sweepStagedLocked(now time.Time) {
	for id, z := range staged.zips {
		if now.After(z.ExpiresAt) {
			delete(staged.zips, id)
			if err := os.Remove(z.Path); err != nil && !os.IsNotExist(err) {
				log.Printf("Failed to remove expired plugin ZIP %s: %v", z.Path, err)
			}
		}
	}
}
