package logs

import "testing"

func TestIsExcludedPage(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"wp-admin root", "/wp-admin", true},
		{"wp-admin subpath", "/wp-admin/edit.php", true},
		{"wp-json root", "/wp-json", true},
		{"wp-json subpath", "/wp-json/wp/v2/posts", true},
		{"regular page", "/about", false},
		{"path containing but not starting with wp-admin", "/blog/wp-admin-tips", false},
		{"homepage", "/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExcludedPage(tt.path); got != tt.want {
				t.Errorf("isExcludedPage(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
