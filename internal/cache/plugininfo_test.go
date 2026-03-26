package cache

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestSanitizePluginInfo_DecodesNameAndStripsAuthorHTML(t *testing.T) {
	info := &models.PluginInfo{
		Slug:   "example-plugin",
		Name:   "My Plugin &#8211; Lite",
		Author: "<a href='https://example.com'>Jane &amp; Co</a>",
	}

	sanitizePluginInfo(info)

	if got, want := info.Name, "My Plugin - Lite"; got != want {
		t.Fatalf("unexpected sanitized name: got %q, want %q", got, want)
	}

	if got, want := info.Author, "Jane & Co"; got != want {
		t.Fatalf("unexpected sanitized author: got %q, want %q", got, want)
	}
}

func TestSanitizePluginInfo_TrimWhitespace(t *testing.T) {
	info := &models.PluginInfo{
		Slug:   "example-plugin",
		Name:   "  Plugin&nbsp;Name  ",
		Author: "  <strong> ACME&nbsp;Inc </strong>  ",
	}

	sanitizePluginInfo(info)

	if got, want := info.Name, "Plugin Name"; got != want { // contains NBSP
		t.Fatalf("unexpected trimmed/decoded name: got %q, want %q", got, want)
	}

	if got, want := info.Author, "ACME Inc"; got != want { // contains NBSP
		t.Fatalf("unexpected trimmed/decoded author: got %q, want %q", got, want)
	}
}

func TestSanitizePluginInfo_NilIsNoop(t *testing.T) {
	// Should not panic.
	sanitizePluginInfo(nil)
}
