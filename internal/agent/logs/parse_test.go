package logs

import "testing"

func TestParseLine(t *testing.T) {
	line := `185.111.111.156 - - [17/Aug/2026:00:16:22 +0300] "GET /?jman_cache_bypass=1786914982314384469 HTTP/2.0" 302 0 "-" "jman/1.0 (WordPress Management Tool)"`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine() error = %v", err)
	}

	if entry.RemoteAddr != "185.111.111.156" {
		t.Errorf("RemoteAddr = %q, want %q", entry.RemoteAddr, "185.111.111.156")
	}
	if entry.Method != "GET" {
		t.Errorf("Method = %q, want %q", entry.Method, "GET")
	}
	if entry.Path != "/?jman_cache_bypass=1786914982314384469" {
		t.Errorf("Path = %q", entry.Path)
	}
	if entry.Status != 302 {
		t.Errorf("Status = %d, want 302", entry.Status)
	}
	if entry.UserAgent != "jman/1.0 (WordPress Management Tool)" {
		t.Errorf("UserAgent = %q", entry.UserAgent)
	}
	if !entry.Time.Equal(entry.Time) {
		t.Errorf("Time not parsed")
	}
	if got := entry.Time.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-08-16T21:16:22Z" {
		t.Errorf("Time (UTC) = %s, want 2026-08-16T21:16:22Z (17/Aug 00:16:22 +0300)", got)
	}
}

func TestParseLine_WithReferer(t *testing.T) {
	line := `185.111.111.154 - - [17/Aug/2026:00:16:22 +0300] "GET /wp-login.php?redirect_to=https%3A%2F%2Fkehitys.jcore.fi%2F%3Fjman_cache_bypass%3D1786914982314384469 HTTP/2.0" 200 1686 "https://kehitys.jcore.fi/?jman_cache_bypass=1786914982314384469" "jman/1.0 (WordPress Management Tool)"`

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine() error = %v", err)
	}
	if entry.Referer != "https://kehitys.jcore.fi/?jman_cache_bypass=1786914982314384469" {
		t.Errorf("Referer = %q", entry.Referer)
	}
	if entry.Status != 200 {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
}

func TestParseLine_Malformed(t *testing.T) {
	if _, err := ParseLine("not a log line"); err == nil {
		t.Error("expected error for malformed line")
	}
}

func TestPathWithoutQuery(t *testing.T) {
	cases := map[string]string{
		"/?jman_cache_bypass=123": "/",
		"/wp-json":                "/wp-json",
		"/a/b?x=1&y=2":            "/a/b",
	}
	for in, want := range cases {
		if got := PathWithoutQuery(in); got != want {
			t.Errorf("PathWithoutQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsInternalTraffic(t *testing.T) {
	internal := Entry{UserAgent: "jman/1.0 (WordPress Management Tool)", Path: "/"}
	if !IsInternalTraffic(internal) {
		t.Error("expected jman UA to be classified as internal traffic")
	}

	byQueryParam := Entry{UserAgent: "Mozilla/5.0", Path: "/?jman_cache_bypass=123"}
	if !IsInternalTraffic(byQueryParam) {
		t.Error("expected jman_cache_bypass query param to be classified as internal traffic")
	}

	real := Entry{UserAgent: "Mozilla/5.0 (Linux; Android 10; K)", Path: "/"}
	if IsInternalTraffic(real) {
		t.Error("expected real browser UA not to be classified as internal traffic")
	}
}

func TestIsBotUserAgent(t *testing.T) {
	if !IsBotUserAgent("Mozilla/5.0 (compatible; FacebookBot/1.0; +https://developers.facebook.com/docs/sharing/webmasters/crawler)") {
		t.Error("expected FacebookBot UA to be classified as a bot")
	}
	if IsBotUserAgent("Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Mobile Safari/537.36") {
		t.Error("expected real mobile browser UA not to be classified as a bot")
	}
}
