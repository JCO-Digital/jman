package hetzner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/inventory"
)

const serverJSON = `{
  "id": %d, "name": "%s", "status": "running", "created": "2024-03-06T09:30:39+00:00",
  "public_net": {"ipv4": {"ip": "203.0.113.%d"}, "ipv6": {"ip": "2001:db8::/64"}},
  "server_type": {"name": "cx32", "cores": 4, "memory": 8.0, "disk": 80, "cpu_type": "shared", "architecture": "x86"},
  "location": {"name": "hel1", "city": "Helsinki", "country": "FI"},
  "image": {"name": "ubuntu-24.04", "description": "Ubuntu 24.04", "os_flavor": "ubuntu", "os_version": "24.04"},
  "primary_disk_size": 80, "backup_window": null, "labels": {"env": "prod"},
  "protection": {"delete": true, "rebuild": true},
  "outgoing_traffic": 123456
}`

func newTestProvider(t *testing.T, tokens ...string) (*Provider, *[]string) {
	t.Helper()
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		requests = append(requests, auth+" "+r.URL.RawQuery)
		switch auth {
		case "Bearer project-a":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprintf(w, `{"servers": [`+serverJSON+`], "meta": {"pagination": {"page": 1, "next_page": 2}}}`, 1, "web1.example.com", 1)
				return
			}
			fmt.Fprintf(w, `{"servers": [`+serverJSON+`], "meta": {"pagination": {"page": 2, "next_page": null}}}`, 2, "web2.example.com", 2)
		case "Bearer project-b":
			fmt.Fprintf(w, `{"servers": [{"id": 3, "name": "v6only", "status": "off", "public_net": {"ipv4": null, "ipv6": {"ip": "2001:db8:1::/64"}}, "server_type": {"name": "cax11"}, "location": {"name": "fsn1"}, "image": null}], "meta": {"pagination": {"next_page": null}}}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error": {"code": "unauthorized", "message": "unable to authenticate"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return &Provider{BaseURL: srv.URL, HTTP: srv.Client(), Tokens: func() []string { return tokens }}, &requests
}

func TestFetchAllProjectsAndPages(t *testing.T) {
	p, requests := newTestProvider(t, "project-a", "", "project-b", "project-a")
	items, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3 (two pages of project A, project B, A's repeat de-duplicated)", len(items))
	}
	if !strings.Contains((*requests)[0], "per_page=50") {
		t.Errorf("first request: %s", (*requests)[0])
	}

	web1 := items[0]
	if web1.Type != "server" || web1.ProviderType != "cloud_server" || web1.ProviderID != "1" || web1.Name != "web1.example.com" {
		t.Errorf("item: %+v", web1)
	}
	d := web1.Data
	if d["ipv4"] != "203.0.113.1" || d["server_type"] != "cx32" || d["memory_gb"] != 8.0 || d["city"] != "Helsinki" || d["image"] != "ubuntu-24.04" || d["delete_protection"] != true {
		t.Errorf("data: %+v", d)
	}
	for _, key := range []string{"id", "name", "outgoing_traffic", "backup_window"} {
		if _, ok := d[key]; ok {
			t.Errorf("data has %q: %+v", key, d)
		}
	}

	// IPv6-only server without an image.
	v6 := items[2].Data
	if _, ok := v6["ipv4"]; ok || v6["ipv6"] != "2001:db8:1::/64" {
		t.Errorf("v6-only data: %+v", v6)
	}
	if _, ok := v6["image"]; ok {
		t.Errorf("image without an image: %+v", v6)
	}
}

func TestFetchErrors(t *testing.T) {
	p, _ := newTestProvider(t)
	if _, err := p.Fetch(context.Background()); !errors.Is(err, inventory.ErrNotConfigured) {
		t.Errorf("no tokens: %v", err)
	}

	// One bad project fails the whole fetch, so the sync removes nothing.
	p, _ = newTestProvider(t, "project-a", "revoked")
	_, err := p.Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "project 2 of 2") || !strings.Contains(err.Error(), "unable to authenticate") {
		t.Errorf("bad token: %v", err)
	}
	if strings.Contains(err.Error(), "revoked") {
		t.Errorf("error leaks the token: %v", err)
	}
}
