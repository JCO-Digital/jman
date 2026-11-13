package domainlookup

import (
	"errors"
	"strings"
	"testing"
)

// Trimmed real answers of each configured WHOIS server, for the registries'
// own domains.
var whoisSamples = map[string]string{
	"se": `# Copyright (c) 1997- The Swedish Internet Foundation.
state:            active
domain:           iis.se
holder:           stifte6931-00001
expires:          2031-03-09
nserver:          nsa.dnsnode.net
nserver:          nsp.dnsnode.net
status:           ok
registrar:        Internetstift
`,
	"ee": `Estonia .ee Top Level Domain WHOIS server

Domain:
name:       internet.ee
status:     ok (paid and in zone)
expire:     2030-02-05
outzone:    
delete:     

Registrant:
name:       Eesti Interneti Sihtasutus
org id:     90010019

Registrar:
name:       Zone Media OÜ
url:        http://www.zone.ee

Name servers:
nserver:   b.tld.ee
nserver:   c.tld.ee
changed:   2019-11-07 12:33:55 +02:00
`,
	"dk": `# Hello 192.0.2.1. Your session has been logged.
Domain:               punktum.dk
DNS:                  punktum.dk
Registered:           2018-01-25
Expires:              2027-01-31
Status:               Active

Nameservers
Hostname:             auth01.ns.dk-hostmaster.dk
Hostname:             auth02.ns.dk-hostmaster.dk
`,
	"io": `Domain Name: nic.io
Registrar WHOIS Server: whois.identitydigital.services
Registry Expiry Date: 2027-05-01T00:00:02Z
Registrar: Registry Operator acts as Registrar (9999)
Registrar IANA ID: 9999
Name Server: b0.nic.io
Name Server: a0.nic.io
>>> Last update of WHOIS database: 2026-11-13T10:00:01Z <<<
`,
	"eu": `% The WHOIS service offered by EURid
Domain: eurid.eu
Script: LATIN

Registrant:
        NOT DISCLOSED!
        Visit www.eurid.eu for the web-based WHOIS.

Technical:
        Organisation: EURid vzw
        Email: tech@eurid.eu

Registrar:
        Name: EURid vzw
        Website: https://www.eurid.eu

Name servers:
        nsx.eurid.eu (185.151.141.1)
        nsx.eurid.eu (2a02:568:fe00::6575)
        ns1.eurid.eu (2001:67c:9c:3937::252)
        nsp.netnod.se

Please visit www.eurid.eu for more info.
`,
}

func TestParseWhoisPerRegistry(t *testing.T) {
	cases := []struct {
		tld, registrar, expires, nameservers string
	}{
		{"se", "Internetstift", "2031-03-09", "nsa.dnsnode.net,nsp.dnsnode.net"},
		{"ee", "Zone Media OÜ", "2030-02-05", "b.tld.ee,c.tld.ee"},
		{"dk", "", "2027-01-31", "auth01.ns.dk-hostmaster.dk,auth02.ns.dk-hostmaster.dk"},
		{"io", "Registry Operator acts as Registrar (9999)", "2027-05-01", "a0.nic.io,b0.nic.io"},
		{"eu", "EURid vzw", "", "ns1.eurid.eu,nsp.netnod.se,nsx.eurid.eu"},
	}
	for _, c := range cases {
		var res Result
		if err := parseWhois(whoisSamples[c.tld], whoisServers[c.tld], &res); err != nil {
			t.Errorf(".%s: %v", c.tld, err)
			continue
		}
		expires := ""
		if res.ExpiresAt != nil {
			expires = res.ExpiresAt.Format("2006-01-02")
		}
		if res.Registrar != c.registrar || expires != c.expires || strings.Join(res.Nameservers, ",") != c.nameservers {
			t.Errorf(".%s: registrar=%q expires=%q ns=%v", c.tld, res.Registrar, expires, res.Nameservers)
		}
	}
	// .me and .ie answer in the same gTLD style as .io.
	for _, tld := range []string{"me", "ie"} {
		var res Result
		if err := parseWhois(whoisSamples["io"], whoisServers[tld], &res); err != nil || res.ExpiresAt == nil {
			t.Errorf(".%s: %+v %v", tld, res, err)
		}
	}
}

func TestParseWhoisNotFound(t *testing.T) {
	var res Result
	err := parseWhois("% Copyright\ndomain \"nosuchdomain.se\" not found\n", whoisServers["se"], &res)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
	if err := parseWhois("Requests of this client are not permitted.\n", whoisServers["se"], &res); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("refused: %v", err)
	}
}

// Every TLD jman has domains under is covered by RDAP, a configured RDAP
// server or WHOIS; this lists the ones RDAP's bootstrap file lacks.
func TestRemainingTLDsConfigured(t *testing.T) {
	for _, tld := range []string{"se", "ee", "io", "ie", "eu", "me", "dk"} {
		if _, ok := whoisServers[tld]; !ok {
			t.Errorf(".%s has no WHOIS server", tld)
		}
	}
	if rdapServers["ch"] == "" {
		t.Error(".ch has no RDAP server")
	}
}
