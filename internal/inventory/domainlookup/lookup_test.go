package domainlookup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A .com answer in the shape Verisign sends: registrar as a vCard entity.
const rdapCom = `{
  "ldhName": "EXAMPLE.COM",
  "events": [
    {"eventAction": "registration", "eventDate": "2000-03-03T12:13:23Z"},
    {"eventAction": "expiration", "eventDate": "2033-03-03T12:13:23Z"}
  ],
  "nameservers": [{"ldhName": "NS2.EXAMPLE.COM"}, {"ldhName": "NS1.EXAMPLE.COM."}],
  "entities": [{
    "roles": ["registrar"],
    "handle": "292",
    "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "Example Registrar Inc."]]]
  }]
}`

// A .fi answer in the shape rdap.fi sends: no registrar entity.
const rdapFi = `{
  "ldhName": "example.fi",
  "events": [{"eventAction": "expiration", "eventDate": "2027-08-31T00:00:00+03:00"}],
  "nameservers": [{"ldhName": "ns1.example.net"}, {"ldhName": "ns2.example.net"}],
  "entities": [{"roles": ["registrant"], "vcardArray": ["vcard", [["fn", {}, "text", "Example Oy"]]]}]
}`

// A whois.fi answer, trimmed to the lines that matter.
const whoisFi = `domain.............: example.fi
status.............: Registered
expires............: 31.8.2027 00:00:00
Nameservers
nserver............: ns1.example.net [OK]
nserver............: ns2.example.net [OK]
Registrar
registrar..........: Example Registrar Oy
>>> Last update of WHOIS database: 13.11.2026 9:45:25 (EET) <<<
`

type fakeRegistry struct {
	rdap         *httptest.Server
	whoisQueries atomic.Int32
	whoisAnswer  string
}

func newTestClient(t *testing.T) (*Client, *fakeRegistry) {
	t.Helper()
	reg := &fakeRegistry{whoisAnswer: whoisFi}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/dns.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"services": [[["com", "net"], ["%[1]s/com/"]], [["fi"], ["%[1]s/fi"]]]}`, srv.URL)
	})
	mux.HandleFunc("/com/domain/example.com", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/rdap+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		fmt.Fprint(w, rdapCom)
	})
	mux.HandleFunc("/fi/domain/example.fi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, rdapFi)
	})
	mux.HandleFunc("/fi/domain/xn--hkkinen-5wa.fi", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, rdapFi)
	})
	srv = httptest.NewServer(mux) // unknown paths answer 404
	t.Cleanup(srv.Close)
	reg.rdap = srv

	c := NewClient()
	c.HTTP = srv.Client()
	c.BootstrapURL = srv.URL + "/dns.json"
	c.MinInterval = 0
	c.WhoisServers = map[string]WhoisServer{
		"fi": whoisServers["fi"],
		"se": {Address: "whois.example:43", Registrar: []string{"registrar"}, Expires: []string{"expires"}, Nameservers: []string{"nserver"}, ExpiresLayouts: []string{"2006-01-02"}},
	}
	c.Dial = func(ctx context.Context, address string) (net.Conn, error) {
		reg.whoisQueries.Add(1)
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			if _, err := bufio.NewReader(server).ReadString('\n'); err != nil {
				return
			}
			_, _ = server.Write([]byte(reg.whoisAnswer))
		}()
		return client, nil
	}
	return c, reg
}

func TestLookupRDAPComplete(t *testing.T) {
	c, reg := newTestClient(t)
	res, err := c.Lookup(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "rdap" || res.Registrar != "Example Registrar Inc." {
		t.Errorf("result: %+v", res)
	}
	if got := strings.Join(res.Nameservers, ","); got != "ns1.example.com,ns2.example.com" {
		t.Errorf("nameservers = %s", got)
	}
	if res.ExpiresAt == nil || !res.ExpiresAt.Equal(time.Date(2033, 3, 3, 12, 13, 23, 0, time.UTC)) {
		t.Errorf("expires = %v", res.ExpiresAt)
	}
	if n := reg.whoisQueries.Load(); n != 0 {
		t.Errorf("WHOIS asked %d times for a complete RDAP answer", n)
	}
}

func TestLookupFillsRegistrarFromWhois(t *testing.T) {
	c, reg := newTestClient(t)
	res, err := c.Lookup(context.Background(), "example.fi")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "rdap+whois" || res.Registrar != "Example Registrar Oy" {
		t.Errorf("result: %+v", res)
	}
	// RDAP's expiry (with its zone) wins over WHOIS's zoneless one.
	if res.ExpiresAt == nil || !res.ExpiresAt.Equal(time.Date(2027, 8, 30, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("expires = %v", res.ExpiresAt)
	}
	if reg.whoisQueries.Load() != 1 {
		t.Errorf("WHOIS queries = %d", reg.whoisQueries.Load())
	}

	// A WHOIS failure keeps the RDAP data.
	reg.whoisAnswer = "Too many queries, try again later\n"
	res, err = c.Lookup(context.Background(), "example.fi")
	if err != nil || res.Source != "rdap" || res.Registrar != "" || len(res.Nameservers) != 2 {
		t.Errorf("WHOIS failure: %+v %v", res, err)
	}
}

func TestLookupUnicodeDomain(t *testing.T) {
	c, _ := newTestClient(t)
	if _, err := c.Lookup(context.Background(), "häkkinen.fi"); err != nil {
		t.Fatalf("Unicode name not sent as Punycode: %v", err)
	}
}

func TestLookupNotFoundAndUnsupported(t *testing.T) {
	c, _ := newTestClient(t)
	if _, err := c.Lookup(context.Background(), "missing.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing domain: %v", err)
	}
	if _, err := c.Lookup(context.Background(), "example.dk"); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), ".dk") {
		t.Errorf("unsupported TLD: %v", err)
	}
}

func TestLookupWhoisOnly(t *testing.T) {
	c, reg := newTestClient(t)
	reg.whoisAnswer = "registrar: Example AB\nexpires: 2028-01-15\nnserver: ns1.example.se\n"
	res, err := c.Lookup(context.Background(), "example.se")
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != "whois" || res.Registrar != "Example AB" || res.ExpiresAt == nil || res.ExpiresAt.Format("2006-01-02") != "2028-01-15" {
		t.Errorf("result: %+v", res)
	}
}

func TestPaceSpacesRequestsPerHost(t *testing.T) {
	c := &Client{MinInterval: 50 * time.Millisecond}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := c.pace(context.Background(), "a.example"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.pace(context.Background(), "b.example"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 100*time.Millisecond || d > time.Second {
		t.Errorf("3 requests to one host took %v, want about 100ms", d)
	}
}

func TestDecodeWhois(t *testing.T) {
	latin1 := []byte("registrar: Liikenne- ja viestint\xe4virasto")
	if got := decodeWhois(latin1); got != "registrar: Liikenne- ja viestintävirasto" {
		t.Errorf("Latin-1: %q", got)
	}
	if got := decodeWhois([]byte("registrar: Häme Oy")); got != "registrar: Häme Oy" {
		t.Errorf("UTF-8: %q", got)
	}
}
