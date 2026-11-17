// Command textnoise measures how much a site's visible text changes between
// fetches when nothing on the site changed, using the same extraction,
// normalization, noise filter and diff as the update text diff
// (internal/textdiff, docs/update-text-diff.md).
//
// Each URL is fetched -n times (default 3) with -delay between fetches,
// past the page cache. For every URL it reports:
//
//   - lines: visible text lines on the first fetch
//   - changed: normalized lines that differ between any fetch and the first
//   - residual: what the update diff would report as false positives. The
//     first two fetches are the baseline and their differences are noise;
//     every later fetch is diffed against the first with that noise hidden.
//
// Usage:
//
//	go run ./scripts/textnoise -f sites.txt
//	go run ./scripts/textnoise -n 4 -delay 60s -v example.com example.org/en/
//
// A longer -delay mimics the time an update job takes, so content that
// changes over time (relative dates, scheduled posts) shows up too.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/JCO-Digital/jman/internal/textdiff"
	"github.com/JCO-Digital/jman/internal/utils"
)

type report struct {
	URL      string `json:"url"`
	Lines    int    `json:"lines"`
	Changed  int    `json:"changed"`
	Residual int    `json:"residual"`
	Redirect bool   `json:"final_url_changed"`
	Error    string `json:"error,omitempty"`
	// Details, kept with -v.
	NoiseLines    []string `json:"noise_lines,omitempty"`
	ResidualLines []string `json:"residual_lines,omitempty"`
}

func main() {
	file := flag.String("f", "", "file with one URL or domain per line (# comments allowed)")
	n := flag.Int("n", 3, "fetches per URL (at least 3 for the residual)")
	delay := flag.Duration("delay", 2*time.Second, "delay between fetches of the same URL")
	conc := flag.Int("c", 4, "URLs measured at once")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout per fetch")
	verbose := flag.Bool("v", false, "print noise and residual lines")
	asJSON := flag.Bool("json", false, "print JSON instead of a table")
	dump := flag.String("dump", "", "directory to write each fetch's extracted lines to")
	flag.Parse()

	urls, err := readURLs(*file, flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "no URLs: pass them as arguments or with -f")
		flag.Usage()
		os.Exit(2)
	}
	if *n < 2 {
		fmt.Fprintln(os.Stderr, "-n must be at least 2")
		os.Exit(2)
	}
	if *dump != "" {
		if err := os.MkdirAll(*dump, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	m := measurer{client: utils.NewHTTPClient(*timeout), n: *n, delay: *delay, dump: *dump}
	reports := make([]report, len(urls))
	sem := make(chan struct{}, max(*conc, 1))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			reports[i] = m.measure(u, i)
			fmt.Fprintf(os.Stderr, "done %s\n", u)
		}()
	}
	wg.Wait()

	if !*verbose {
		for i := range reports {
			reports[i].NoiseLines, reports[i].ResidualLines = nil, nil
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(reports)
		return
	}
	printTable(reports, *n)
}

func readURLs(file string, args []string) ([]string, error) {
	var raw []string
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			raw = append(raw, sc.Text())
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	raw = append(raw, args...)
	var urls []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.Contains(s, "://") {
			s = "https://" + s
		}
		if !strings.Contains(strings.SplitN(s, "://", 2)[1], "/") {
			s += "/"
		}
		urls = append(urls, s)
	}
	return urls, nil
}

type measurer struct {
	client *http.Client
	n      int
	delay  time.Duration
	dump   string
}

func (m measurer) measure(url string, idx int) report {
	r := report{URL: url}
	var pages []textdiff.Page
	for i := 0; i < m.n; i++ {
		if i > 0 {
			time.Sleep(m.delay)
		}
		p := textdiff.Fetch(context.Background(), m.client, url, nil)
		if p.Error != "" {
			r.Error = p.Error
			return r
		}
		if m.dump != "" {
			name := fmt.Sprintf("%s/%03d-%s-%d.txt", m.dump, idx, slug(url), i)
			_ = os.WriteFile(name, []byte(strings.Join(p.Lines, "\n")+"\n"), 0o644)
		}
		pages = append(pages, p)
	}

	base := pages[0].Lines
	r.Lines = len(base)
	changed := map[string]bool{}
	for _, p := range pages[1:] {
		if p.FinalURL != pages[0].FinalURL {
			r.Redirect = true
		}
		for _, h := range textdiff.Diff(base, p.Lines, nil) {
			addChanges(changed, h)
		}
	}
	r.Changed = len(changed)

	noise := textdiff.Noise(base, pages[1].Lines)
	hide := func(_, norm string) bool { return noise[norm] }
	residual := map[string]bool{}
	for _, p := range pages[2:] {
		for _, h := range textdiff.Diff(base, p.Lines, hide) {
			addChanges(residual, h)
		}
	}
	r.Residual = len(residual)
	r.NoiseLines = sortedKeys(noise)
	r.ResidualLines = sortedKeys(residual)
	return r
}

func addChanges(set map[string]bool, h textdiff.Hunk) {
	for _, l := range h.Lines {
		if l.Kind != textdiff.KindContext {
			set[l.Text] = true
		}
	}
}

func printTable(reports []report, n int) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "URL\tLINES\tCHANGED\tRESIDUAL\tNOTE")
	quiet, measured := 0, 0
	for _, r := range reports {
		if r.Error != "" {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t%s\n", r.URL, r.Error)
			continue
		}
		measured++
		if r.Residual == 0 {
			quiet++
		}
		note := ""
		if r.Redirect {
			note = "final URL changed"
		}
		res := fmt.Sprint(r.Residual)
		if n < 3 {
			res = "n/a"
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n", r.URL, r.Lines, r.Changed, res, note)
	}
	tw.Flush()
	if n >= 3 && measured > 0 {
		fmt.Printf("\n%d of %d measured URLs would show no false positive (residual 0).\n", quiet, measured)
	}
	for _, r := range reports {
		if len(r.NoiseLines) == 0 && len(r.ResidualLines) == 0 {
			continue
		}
		fmt.Printf("\n== %s\n", r.URL)
		for _, l := range r.NoiseLines {
			fmt.Printf("  noise     %s\n", trunc(l))
		}
		for _, l := range r.ResidualLines {
			fmt.Printf("  RESIDUAL  %s\n", trunc(l))
		}
	}
}

func trunc(s string) string {
	if r := []rune(s); len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return s
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var nonSlug = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slug(u string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.SplitN(u, "://", 2)[1], "-"), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
