#!/usr/bin/env bash
# Sets up this container as a SpinupWP-like server: for each domain in
# $SITES a Unix user, /sites/<domain>/{files,logs}, a database, a PHP-FPM
# pool, an nginx vhost, a WordPress install and SSH logins. Then runs
# jman-agent, unless NO_AGENT is set (a server jman-api reaches over SSH
# only). Idempotent: the /sites volume and the database survive container
# rebuilds.
set -euo pipefail

: "${SITES:?}" "${DB_ROOT_PASSWORD:?}"
DB_HOST="${DB_HOST:-mariadb}"
PHP_VERSION="$(ls /etc/php)"
FPM_POOLS="/etc/php/$PHP_VERSION/fpm/pool.d"
export LC_ALL=C

log() { echo "[server] $*"; }

# Plugins per site user, as slug[:version]. The pinned versions are old
# releases with published vulnerabilities, so jman finds real exposures:
# duplicator 1.3.26 (CVE-2020-11738, in CISA KEV), contact-form-7 5.3.1
# (CVE-2020-35489, CVSS 10), wp-statistics 13.0.7, litespeed-cache 6.3.0.1,
# really-simple-ssl 9.0.0 and forminator 1.24.6 (CVSS 9.8 and up), and
# wordfence 7.5.9 (one CVSS 4.8). All of them keep wp-cli's output clean on
# PHP 8.4 (elementor 3.6 doesn't). The containers' web servers aren't
# published on the host. A plugin is only installed when it is missing, so an
# update made through jman stays after a restart; make local-dev-reset brings
# the old versions back.
site_plugins() {
	case "$1" in
	alpha) echo "classic-editor duplicator:1.3.26 contact-form-7:5.3.1" ;;
	beta) echo "classic-editor wp-statistics:13.0.7 litespeed-cache:6.3.0.1 contact-form-7:5.3.1" ;;
	gamma) echo "classic-editor really-simple-ssl:9.0.0 forminator:1.24.6 wordfence:7.5.9" ;;
	delta) echo "classic-editor contact-form-7:5.3.1 litespeed-cache:6.3.0.1" ;;
	*) echo "classic-editor" ;;
	esac
}

# The WordPress version a new site starts on: gamma runs an older release,
# for core vulnerabilities.
site_core() {
	case "$1" in
	gamma) echo "6.8.2" ;;
	*) echo "latest" ;;
	esac
}

as_user() { # user cmd...
	local user="$1"
	shift
	runuser -u "$user" -- env -i PATH=/usr/local/bin:/usr/bin:/bin HOME="$(getent passwd "$user" | cut -d: -f6)" \
		WP_CLI_CACHE_DIR=/var/cache/wp-cli "$@"
}

ensure_user() { # user home
	id "$1" >/dev/null 2>&1 && return
	if [[ -d "$2" ]]; then
		# Keep the owner of a site that survived a container rebuild.
		local uid
		uid="$(stat -c %u "$2")"
		groupadd -g "$uid" "$1"
		useradd -u "$uid" -g "$uid" -d "$2" -s /bin/bash "$1"
	else
		useradd -U -d "$2" -m -s /bin/bash "$1"
	fi
}

# A few hours of past visitor traffic, so the agent has whole hours to report
# right away (it only sends an hour once it has ended).
backfill_traffic() { # log
	local h i ua path
	local uas=("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1"
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	local paths=(/ / / /sample-page/ /hello-world/ /wp-login.php)
	for h in 6 5 4 3 2 1; do
		for ((i = 0; i < 8 + RANDOM % 12; i++)); do
			ua="${uas[RANDOM % 3]}"
			path="${paths[RANDOM % 6]}"
			printf '198.51.100.%d - - [%s] "GET %s HTTP/1.1" 200 %d "https://www.example.org/" "%s"\n' \
				$((RANDOM % 50 + 1)) "$(date -d "-$h hour +$((i * 3)) min" '+%d/%b/%Y:%H:%M:%S %z')" \
				"$path" $((RANDOM % 9000 + 1000)) "$ua"
		done
	done >>"$1"
}

setup_site() { # domain
	local domain="$1" user="${1%%.*}" home="/sites/$1"
	local db="wp_${user}" dbpass="localdev-${user}"
	log "Setting up $domain (user $user)"
	ensure_user "$user" "$home"
	mkdir -p "$home/files" "$home/logs"
	chown "$user:$user" "$home" "$home/files" "$home/logs"
	chmod 750 "$home"
	# nginx (www-data) serves the files; SpinupWP adds it to the site group.
	usermod -aG "$user" www-data

	mariadb -h "$DB_HOST" -uroot -p"$DB_ROOT_PASSWORD" <<-SQL
		CREATE DATABASE IF NOT EXISTS \`$db\`;
		CREATE USER IF NOT EXISTS '$user'@'%' IDENTIFIED BY '$dbpass';
		GRANT ALL ON \`$db\`.* TO '$user'@'%';
	SQL

	cat >"$FPM_POOLS/$domain.conf" <<-EOF
		[$domain]
		user = $user
		group = $user
		listen = /run/php/$domain.sock
		listen.owner = www-data
		listen.group = www-data
		pm = ondemand
		pm.max_children = 3
	EOF

	cat >"/etc/nginx/sites-enabled/$domain" <<-EOF
		server {
			listen 80;
			server_name $domain;
			root $home/files;
			index index.php;
			access_log $home/logs/access.log;
			error_log $home/logs/error.log;
			# As on SpinupWP: never run PHP from uploads.
			location ~* /wp-content/uploads/.*\.php\$ { deny all; }
			location / { try_files \$uri \$uri/ /index.php?\$args; }
			location ~ \.php\$ {
				include snippets/fastcgi-php.conf;
				fastcgi_pass unix:/run/php/$domain.sock;
			}
		}
	EOF

	local wp=(as_user "$user" wp --path="$home/files")
	if [[ ! -f "$home/files/wp-config.php" ]]; then
		# Not `wp core download`: its PHP tar extractor truncates the long
		# paths in wp-includes/php-ai-client (WordPress 7), which the scan
		# then rightly flags as unknown core files.
		local core tarball url
		core="$(site_core "$user")"
		tarball="/var/cache/wp-cli/wordpress-$core.tar.gz"
		url="https://wordpress.org/wordpress-$core.tar.gz"
		[[ "$core" == latest ]] && url=https://wordpress.org/latest.tar.gz
		if [[ ! -s "$tarball" ]]; then
			curl -fsSL -o "$tarball.$HOSTNAME" "$url"
			mv "$tarball.$HOSTNAME" "$tarball"
		fi
		as_user "$user" tar -xzf "$tarball" -C "$home/files" --strip-components=1
		"${wp[@]}" config create --dbname="$db" --dbuser="$user" --dbpass="$dbpass" --dbhost="$DB_HOST" --skip-check
	fi
	if ! "${wp[@]}" core is-installed 2>/dev/null; then
		"${wp[@]}" core install --url="http://$domain" --title="$domain (local dev)" \
			--admin_user=admin --admin_password=localdev --admin_email="admin@$domain" --skip-email
		# Don't let core update itself (the older one is on purpose).
		"${wp[@]}" config set WP_AUTO_UPDATE_CORE false --raw
		backfill_traffic "$home/logs/access.log"
		chown "$user:$user" "$home/logs/access.log"
	fi

	local entry slug version
	for entry in $(site_plugins "$user"); do
		slug="${entry%%:*}"
		version="${entry#"$slug"}"
		version="${version#:}"
		"${wp[@]}" plugin is-installed "$slug" 2>/dev/null && continue
		"${wp[@]}" plugin install "$slug" ${version:+--version="$version"} ||
			{ log "Couldn't install $slug ${version:-(latest)} on $domain"; continue; }
		"${wp[@]}" plugin activate "$slug" || log "Couldn't activate $slug on $domain"
	done

	# SSH as the site user with the local-dev key (see sshd.conf).
	if [[ -s /run/local-dev/ssh.pub ]]; then
		install -m 0644 /run/local-dev/ssh.pub "/sites/.local-dev-ssh/authorized_keys/$user"
	fi
}

setup_ssh() {
	mkdir -p -m 0755 /sites/.local-dev-ssh /sites/.local-dev-ssh/authorized_keys /run/sshd
	chown root:root /sites/.local-dev-ssh /sites/.local-dev-ssh/authorized_keys
	chmod 0755 /sites/.local-dev-ssh /sites/.local-dev-ssh/authorized_keys
	[[ -f /sites/.local-dev-ssh/ssh_host_ed25519_key ]] ||
		ssh-keygen -q -t ed25519 -N "" -C "$HOSTNAME" -f /sites/.local-dev-ssh/ssh_host_ed25519_key
	[[ -s /run/local-dev/ssh.pub ]] || log "No /run/local-dev/ssh.pub: SSH logins won't work"
}

mkdir -p /run/php /var/cache/wp-cli/core /var/cache/wp-cli/plugin
# Shared by every site user (and both containers).
chmod 1777 /var/cache/wp-cli /var/cache/wp-cli/core /var/cache/wp-cli/plugin
rm -f "$FPM_POOLS/www.conf"

log "Waiting for MariaDB at $DB_HOST..."
until mariadb -h "$DB_HOST" -uroot -p"$DB_ROOT_PASSWORD" -e 'SELECT 1' >/dev/null 2>&1; do sleep 1; done

setup_ssh
for domain in $SITES; do setup_site "$domain"; done

"/usr/sbin/php-fpm$PHP_VERSION" -D
nginx
/usr/sbin/sshd

# Some live requests through nginx, as on a real server.
for domain in $SITES; do
	for path in / /sample-page/ /wp-login.php; do
		curl -fsS -o /dev/null -H "Host: $domain" -A "Mozilla/5.0 (local-dev)" "http://127.0.0.1$path" || true
	done
done

if [[ -n "${NO_AGENT:-}" ]]; then
	log "Ready; no jman-agent on this server (SSH only)"
	exec sleep infinity
fi
log "Starting jman-agent (reports to $JMAN_AGENT_API_URL)"
exec /usr/local/bin/jman-agent --service -v
