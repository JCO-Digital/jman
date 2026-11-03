#!/usr/bin/env bash
# Plants or removes files that trigger jman-agent's scan rules on one site:
#
#   local-dev-fixtures plant <domain>
#   local-dev-fixtures clean <domain>
#
# Every fixture is inert: the PHP ones only echo a string (and nginx refuses
# PHP in uploads anyway), the .htaccess is ignored by nginx and prepends a
# file that doesn't exist, and the "dump" is a comment. They match the rules
# by name, place or pattern, nothing more. Originals of the edited core and
# plugin files are kept outside the site, in /sites/.local-dev-fixtures (on
# the /sites volume, so they survive a container rebuild).
set -euo pipefail

action="${1:?usage: local-dev-fixtures plant|clean <domain>}"
domain="${2:?usage: local-dev-fixtures plant|clean <domain>}"
user="${domain%%.*}"
files="/sites/$domain/files"
backup="/sites/.local-dev-fixtures/$domain"
marker="jman local-dev scan fixture"

[[ -f "$files/wp-config.php" ]] || { echo "no WordPress site at $files" >&2; exit 1; }

# Files and directories owned by the site user, so foreign-owner doesn't
# fire too.
put() { # path content
	runuser -u "$user" -- mkdir -p "$(dirname "$1")"
	install -o "$user" -g "$user" -m 0644 /dev/null "$1"
	printf '%s' "$2" >"$1"
}

# Appends a PHP comment to an existing file, keeping its original once.
edit() { # relative-path
	local rel="$1"
	mkdir -p -m 0700 /sites/.local-dev-fixtures
	mkdir -p "$(dirname "$backup/$rel")"
	[[ -f "$backup/$rel" ]] || cp -p "$files/$rel" "$backup/$rel"
	grep -q "$marker" "$files/$rel" || printf '\n// %s (inert comment)\n' "$marker" >>"$files/$rel"
}

restore() { # relative-path
	[[ -f "$backup/$1" ]] && cp -p "$backup/$1" "$files/$1" && rm "$backup/$1"
	return 0
}

UPLOADS_PHP="wp-content/uploads/local-dev/cache-info.php"
ENCODED_PHP="wp-content/uploads/local-dev/encoded.php"
HTACCESS="wp-content/uploads/local-dev/.htaccess"
DUMP="backup-local-dev.sql"
CORE="wp-includes/class-wp-hook.php"
PLUGIN="wp-content/plugins/classic-editor/classic-editor.php"

case "$action" in
plant)
	# uploads-php
	put "$files/$UPLOADS_PHP" "<?php
// $marker: uploads-php. Only prints a string.
echo 'jman-local-dev-fixture';
"
	# suspicious-code (eval-decoded): the base64 decodes to: echo 'jman-local-dev-fixture';
	put "$files/$ENCODED_PHP" "<?php
// $marker: suspicious-code. The encoded string is: echo 'jman-local-dev-fixture';
eval(base64_decode('ZWNobyAnam1hbi1sb2NhbC1kZXYtZml4dHVyZSc7'));
"
	# auto-prepend
	put "$files/$HTACCESS" "# $marker: auto-prepend. nginx ignores .htaccess; the file doesn't exist.
php_value auto_prepend_file /nonexistent/jman-local-dev-fixture.php
"
	# stray-file
	put "$files/$DUMP" "-- $marker: stray-file. Not a real dump.
"
	# core-modified and plugin-modified (need checksums, from the second scan on)
	edit "$CORE"
	[[ -f "$files/$PLUGIN" ]] && edit "$PLUGIN"
	echo "Planted scan fixtures on $domain."
	;;
clean)
	rm -rf "$files/wp-content/uploads/local-dev"
	rm -f "$files/$DUMP"
	restore "$CORE"
	restore "$PLUGIN"
	echo "Removed scan fixtures from $domain."
	;;
*)
	echo "unknown action $action" >&2
	exit 1
	;;
esac
