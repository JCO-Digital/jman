package cache

import (
	"os"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func vulnFixture(name string) models.VulnResponse {
	return models.VulnResponse{Data: &models.VulnData{
		Name:          &name,
		Vulnerability: []models.Vulnerability{{Uuid: "u-1", Name: "XSS"}},
	}}
}

func TestReadCachedVulnerabilitiesReusesAndRereads(t *testing.T) {
	setupCacheTest(t)

	if err := WriteJSONCache("vulnerabilities/foo", vulnFixture("Foo")); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := ReadCachedVulnerabilities("foo")
	if err != nil || first == nil || first.Data == nil || *first.Data.Name != "Foo" {
		t.Fatalf("first read = %+v, %v", first, err)
	}
	again, err := ReadCachedVulnerabilities("foo")
	if err != nil || again != first {
		t.Fatalf("unchanged file decoded again (%p vs %p), err %v", again, first, err)
	}

	// A refresh rewrites the file; the next read must see the new data
	// even within the same second.
	if err := WriteJSONCache("vulnerabilities/foo", vulnFixture("Foo Renamed")); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	path := getCacheFilePath("vulnerabilities/foo")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	updated, err := ReadCachedVulnerabilities("foo")
	if err != nil || updated == nil || *updated.Data.Name != "Foo Renamed" {
		t.Fatalf("read after rewrite = %+v, %v", updated, err)
	}
}

func TestReadCachedVulnerabilitiesMissingErrorAndTraversal(t *testing.T) {
	setupCacheTest(t)

	if resp, err := ReadCachedVulnerabilities("missing"); err == nil || resp != nil {
		t.Errorf("missing file = %+v, %v; want error", resp, err)
	}
	if err := WriteJSONCache("vulnerabilities/broken", models.VulnResponse{Error: 1}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if resp, err := ReadCachedVulnerabilities("broken"); err == nil || resp != nil {
		t.Errorf("feed error = %+v, %v; want error", resp, err)
	}
	if resp, err := ReadCachedVulnerabilities("../../etc/passwd"); err == nil || resp != nil {
		t.Errorf("traversal = %+v, %v; want error", resp, err)
	}
}

func TestReadCachedCoreVulnerabilitiesNormalizes(t *testing.T) {
	setupCacheTest(t)

	data := &models.CoreVulnData{Core: "6.4.1", Vulnerability: []models.Vulnerability{{Uuid: "feed-1", Name: "Core XSS", Source: []models.Source{{Id: "CVE-2024-0001", Name: "CVE-2024-0001"}}}}}
	if err := WriteJSONCache("vulnerabilities/core/6.4.1", models.CoreVulnResponse{Data: data}); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := ReadCachedCoreVulnerabilities("6.4.1")
	if err != nil || resp == nil || resp.Data == nil || len(resp.Data.Vulnerability) != 1 {
		t.Fatalf("read = %+v, %v", resp, err)
	}
	if v := resp.Data.Vulnerability[0]; v.FeedUuid != "feed-1" {
		t.Errorf("FeedUuid = %q, want normalized from feed UUID", v.FeedUuid)
	}
}
