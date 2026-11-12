package utils

import "testing"

func TestApexDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"example.com", "example.com"},
		{"www.example.com", "example.com"},
		{"deep.sub.example.com", "example.com"},
		{"WWW.Example.COM", "example.com"},
		{"example.co.uk", "example.co.uk"},
		{"www.example.co.uk", "example.co.uk"},
		{"example.com.", "example.com"},
	}
	for _, c := range cases {
		got, err := ApexDomain(c.in)
		if err != nil {
			t.Errorf("ApexDomain(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ApexDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, in := range []string{"", "localhost", "203.0.113.10", "."} {
		if _, err := ApexDomain(in); err == nil {
			t.Errorf("ApexDomain(%q) succeeded, want error", in)
		}
	}
}
