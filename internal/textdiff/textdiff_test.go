package textdiff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/utils"
)

func TestExtract(t *testing.T) {
	doc := `<!doctype html><html><head><title>Ignored</title><style>p{}</style></head><body>
<nav><ul>
  <li><a href="/">Etusivu</a></li>
  <li><a href="/en/" aria-label="English"><img src="flag.png" alt="">EN</a></li>
</ul></nav>
<script>var nonce = "abc";</script>
<h1>Welcome  to
  <em>our</em> site</h1>
<p>One<br>Two</p>
<div class="gfield gform_validation_container"><label>LinkedIn</label><input name="input_7"></div>
<form><input type="submit" value="Send"><input type="text" placeholder="Your name"></form>
<!-- comment -->
<svg><text>svg text</text></svg>
</body></html>`
	got, err := Extract(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Etusivu",
		"[aria-label] English",
		"EN",
		"Welcome to our site",
		"One",
		"Two",
		"[value] Send",
		"[placeholder] Your name",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract:\n got %q\nwant %q", got, want)
	}
}

// The Polylang bug that motivated the feature: a broken attribute leaks
// into the visible text.
func TestExtractShowsBrokenAttribute(t *testing.T) {
	before := `<ul><li><a href="/en/" lang="en">EN</a></li></ul>`
	after := `<ul><li><a href="/en/" lang="en"">EN" ></a>EN</li></ul>`
	b, _ := Extract(strings.NewReader(before))
	a, _ := Extract(strings.NewReader(after))
	hunks := Diff(b, a, nil)
	if len(hunks) != 1 || hunks[0].Changes() == 0 {
		t.Fatalf("expected one hunk with changes, got %+v", hunks)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Posted  12 minutes ago":       "Posted # minutes ago",
		"17. marraskuuta 2026":          "#. <d> #",
		"Tuesday, November 17, 2026":    "<d>, <d> #, #",
		"lauantaina klo 10.00":          "<d> klo #.#",
		"måndag 3 mars":                 "<d> # <d>",
		"Price 1 299,00 €":              "Price # #,# €",
		"Monitor setup":                 "Monitor setup", // "mon" only as a word
		"Itsenäisyyspäivä on joulukuun": "Itsenäisyyspäivä on <d>",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoise(t *testing.T) {
	a := []string{"Header", "Quote: Great service", "Updated 5 minutes ago", "Footer"}
	b := []string{"Header", "Quote: Loved it", "Updated 6 minutes ago", "Footer"}
	got := Noise(a, b)
	want := map[string]bool{"Quote: Great service": true, "Quote: Loved it": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Noise = %v, want %v", got, want)
	}
}

func lines(n int, prefix string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = prefix + string(rune('a'+i))
	}
	return out
}

func TestDiffContextAndMerging(t *testing.T) {
	before := lines(20, "l")
	after := append([]string(nil), before...)
	after[3] = "changed d"
	after[6] = "changed g" // 2 lines after the first change: same hunk
	after[15] = "changed p"

	hunks := Diff(before, after, nil)
	if len(hunks) != 2 {
		t.Fatalf("got %d hunks, want 2: %+v", len(hunks), hunks)
	}
	first := hunks[0].Lines
	if first[0].Text != "lb" || first[len(first)-1].Text != "li" {
		t.Errorf("first hunk spans %q..%q, want lb..li", first[0].Text, first[len(first)-1].Text)
	}
	if hunks[0].Changes() != 4 || hunks[1].Changes() != 2 {
		t.Errorf("changes = %d, %d; want 4, 2", hunks[0].Changes(), hunks[1].Changes())
	}
	if got := hunks[1].Lines; got[0].Text != "ln" || got[len(got)-1].Text != "lr" {
		t.Errorf("second hunk spans %q..%q, want ln..lr", got[0].Text, got[len(got)-1].Text)
	}
}

func TestDiffShowsOriginalTextButComparesNormalized(t *testing.T) {
	before := []string{"Posted 3 minutes ago", "EN"}
	after := []string{"Posted 9 minutes ago", `EN" >`}
	hunks := Diff(before, after, nil)
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks", len(hunks))
	}
	want := []Line{
		{KindContext, "Posted 9 minutes ago"},
		{KindRemoved, "EN"},
		{KindAdded, `EN" >`},
	}
	if !reflect.DeepEqual(hunks[0].Lines, want) {
		t.Errorf("lines = %+v, want %+v", hunks[0].Lines, want)
	}
}

func TestDiffHide(t *testing.T) {
	before := []string{"a", "Quote one", "b", "Menu item", "c"}
	after := []string{"a", "Quote two", "b", "c"}
	noise := map[string]bool{"Quote one": true, "Quote two": true}
	hide := func(kind, norm string) bool { return noise[norm] }

	hunks := Diff(before, after, hide)
	if len(hunks) != 1 || hunks[0].Changes() != 1 || !hasLine(hunks[0], KindRemoved, "Menu item") {
		t.Fatalf("hunks = %+v, want only the removed menu item", hunks)
	}

	// Muting a line only in one direction.
	hide = func(kind, norm string) bool { return kind == KindAdded && norm == "Menu item" }
	if hunks := Diff(before[:4], before[:3], hide); len(hunks) != 1 {
		t.Errorf("a mute on added lines hid a removal: %+v", hunks)
	}
	if hunks := Diff(before[:3], before[:4], hide); len(hunks) != 0 {
		t.Errorf("a mute on added lines didn't hide the addition: %+v", hunks)
	}
}

func hasLine(h Hunk, kind, text string) bool {
	for _, l := range h.Lines {
		if l.Kind == kind && l.Text == text {
			return true
		}
	}
	return false
}

func TestDiffIdenticalAndEmpty(t *testing.T) {
	if h := Diff(nil, nil, nil); len(h) != 0 {
		t.Errorf("Diff(nil, nil) = %+v", h)
	}
	if h := Diff([]string{"a", "b"}, []string{"a", "b"}, nil); len(h) != 0 {
		t.Errorf("identical pages gave %+v", h)
	}
	h := Diff(nil, []string{"a"}, nil)
	if len(h) != 1 || !hasLine(h[0], KindAdded, "a") {
		t.Errorf("Diff(nil, [a]) = %+v", h)
	}
}

// Applying the edit script must turn a into b and keep the LCS.
func TestDiffOpsRoundTrip(t *testing.T) {
	cases := [][2][]string{
		{{"a", "b", "c", "d"}, {"a", "x", "c", "d"}},
		{{"a", "b", "c"}, {"c", "b", "a"}},
		{{}, {"a", "b"}},
		{{"a", "b"}, {}},
		{{"x", "a", "b", "y"}, {"a", "z", "b"}},
	}
	for _, c := range cases {
		a, b := c[0], c[1]
		var rebuilt []string
		for _, o := range diffOps(a, b) {
			switch o.kind {
			case '=':
				if a[o.a] != b[o.b] {
					t.Errorf("%q→%q: '=' op pairs %q with %q", a, b, a[o.a], b[o.b])
				}
				rebuilt = append(rebuilt, b[o.b])
			case '+':
				rebuilt = append(rebuilt, b[o.b])
			}
		}
		if len(rebuilt) == 0 {
			rebuilt = []string{}
		}
		if !reflect.DeepEqual(rebuilt, b) {
			t.Errorf("%q→%q: rebuilt %q", a, b, rebuilt)
		}
	}
}

func TestFetch(t *testing.T) {
	var gotQuery, gotHeader string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get(utils.CacheBypassParam)
		gotHeader = r.Header.Get("X-Secret")
		w.Write([]byte(`<html><body><p>Hello</p></body></html>`))
	})
	mux.HandleFunc("/old/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/?lang=en&"+r.URL.RawQuery, http.StatusFound)
	})
	mux.HandleFunc("/broken/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<html><body id="error-page"><p>There has been a critical error on this website.</p></body></html>`))
	})
	mux.HandleFunc("/fatal/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<br />\n<b>Fatal error</b>:  Uncaught Error in /x.php"))
	})
	mux.HandleFunc("/down/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	mux.HandleFunc("/empty/", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	p := Fetch(ctx, srv.Client(), srv.URL+"/", map[string]string{"X-Secret": "s"})
	if p.Error != "" || !reflect.DeepEqual(p.Lines, []string{"Hello"}) {
		t.Errorf("Fetch / = %+v", p)
	}
	if gotQuery == "" || gotHeader != "s" {
		t.Errorf("request had bypass %q, header %q", gotQuery, gotHeader)
	}
	if p.FinalURL != srv.URL+"/" {
		t.Errorf("FinalURL = %q", p.FinalURL)
	}

	if p := Fetch(ctx, srv.Client(), srv.URL+"/old/", nil); p.FinalURL != srv.URL+"/?lang=en" || p.Error != "" {
		t.Errorf("redirected fetch = %+v, want final URL without the bypass parameter", p)
	}

	errs := map[string]string{
		"/broken/": "WordPress error page (HTTP 500)",
		"/fatal/":  "PHP fatal error in page",
		"/down/":   "HTTP 502",
		"/empty/":  "Empty page",
	}
	for path, want := range errs {
		if p := Fetch(ctx, srv.Client(), srv.URL+path, nil); p.Error != want {
			t.Errorf("Fetch %s error = %q, want %q", path, p.Error, want)
		}
	}
}
