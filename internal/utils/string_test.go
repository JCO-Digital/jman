package utils

import (
	"testing"
)

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes basic tags",
			input:    "<p>Hello World</p>",
			expected: "Hello World",
		},
		{
			name:     "unescapes html entities",
			input:    "Check &amp; see &quot;this&quot;",
			expected: `Check & see "this"`,
		},
		{
			name:     "unifies hyphens",
			input:    "word—another–yet--one",
			expected: "word-another-yet-one",
		},
		{
			name:     "trims whitespace",
			input:    "   some text   ",
			expected: "some text",
		},
		{
			name:     "complex nested tags and entities",
			input:    "<div><span>Vulnerability: &lt;script&gt;</span> — fixed</div>",
			expected: "Vulnerability: <script> - fixed",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanHTML(tt.input)
			if result != tt.expected {
				t.Errorf("CleanHTML() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestShowFirstPart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic split",
			input:    "Plugin Name - Vulnerability Description",
			expected: "Plugin Name",
		},
		{
			name:     "split with long dash",
			input:    "Plugin Name — Description",
			expected: "Plugin Name",
		},
		{
			name:     "split with multiple hyphens",
			input:    "Plugin Name -- Description",
			expected: "Plugin Name",
		},
		{
			name:     "no separator",
			input:    "Just a Title",
			expected: "Just a Title",
		},
		{
			name:     "hyphen without spaces is not a separator",
			input:    "My-Plugin - Description",
			expected: "My-Plugin",
		},
		{
			name:     "separator at the end",
			input:    "Title - ",
			expected: "Title",
		},
		{
			name:     "multiple separators",
			input:    "Part 1 - Part 2 - Part 3",
			expected: "Part 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShowFirstPart(tt.input)
			if result != tt.expected {
				t.Errorf("ShowFirstPart() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsValidSlug(t *testing.T) {
	tests := []struct {
		slug     string
		expected bool
	}{
		{"akismet", true},
		{"jetpack", true},
		{"contact-form-7", true},
		{"wordpress-seo", true},
		{"woocommerce", true},
		{"my-plugin-123", true},
		{"Invalid-Slug", false},
		{"invalid_slug", false},
		{"invalid/slug", false},
		{"invalid.slug", false},
		{"", false},
		{"-starting-with-hyphen", true}, // WP allows this usually, though rare
		{"ending-with-hyphen-", true},   // WP allows this usually, though rare
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			result := IsValidSlug(tt.slug)
			if result != tt.expected {
				t.Errorf("IsValidSlug(%q) = %v, want %v", tt.slug, result, tt.expected)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"6.6.1", true},
		{"6.6", true},
		{"6", true},
		{"6.6.1.1", true},
		{"", false},
		{"6.6.1.1.1", false},
		{"6.6.1-beta", false},
		{"../../etc/passwd", false},
		{"6.6.1/", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := IsValidVersion(tt.version)
			if result != tt.expected {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes basic colors",
			input:    "\033[31mRed Text\033[0m",
			expected: "Red Text",
		},
		{
			name:     "removes multiple styles",
			input:    "\033[1;32mBold Green\033[0m and \033[34mBlue\033[0m",
			expected: "Bold Green and Blue",
		},
		{
			name:     "removes gray/bright black",
			input:    "\033[90mGray Text\033[0m",
			expected: "Gray Text",
		},
		{
			name:     "no ansi codes",
			input:    "Normal Text",
			expected: "Normal Text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI() = %q, want %q", result, tt.expected)
			}
		})
	}
}
