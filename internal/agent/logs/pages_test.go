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
		{"wp-content subpath", "/wp-content/uploads/2026/08/photo.jpg", true},
		{"wp-includes subpath", "/wp-includes/js/jquery/jquery.js", true},
		{"wp-login.php", "/wp-login.php", true},
		{"wp-login.php with query", "/wp-login.php?action=lostpassword", true},
		{"wp-cron.php", "/wp-cron.php", true},
		{"wp-load.php", "/wp-load.php", true},
		{"wp-mail.php", "/wp-mail.php", true},
		{"wp-signup.php", "/wp-signup.php", true},
		{"wp-activate.php", "/wp-activate.php", true},
		{"wp-trackback.php", "/wp-trackback.php", true},
		{"wp-comments-post.php", "/wp-comments-post.php", true},
		{"wp-links-opml.php", "/wp-links-opml.php", true},
		{"xmlrpc.php", "/xmlrpc.php", true},
		{"regular page", "/about", false},
		{"path containing but not starting with wp-admin", "/blog/wp-admin-tips", false},
		{"post slug coincidentally starting with wp-", "/wp-migration-guide", false},
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
