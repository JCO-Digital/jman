# A managed server for make local-dev-agent: nginx and PHP-FPM serving
# WordPress sites laid out like SpinupWP (/sites/<domain>/{files,logs}, one
# Unix user per site, SSH logins as the site user), with jman-agent built from
# this checkout.
FROM debian:trixie-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		ca-certificates curl less procps mariadb-client nginx openssh-server unzip \
		php-cli php-fpm php-mysql php-xml php-mbstring php-curl php-zip php-gd php-intl \
	&& rm -rf /var/lib/apt/lists/* /etc/nginx/sites-enabled/default /etc/ssh/ssh_host_*

# jman-agent only runs a root-owned wp in a system directory.
ARG WP_CLI_VERSION=2.12.0
RUN curl -fsSL -o /usr/local/bin/wp \
		"https://github.com/wp-cli/wp-cli/releases/download/v${WP_CLI_VERSION}/wp-cli-${WP_CLI_VERSION}.phar" \
	&& chmod 0755 /usr/local/bin/wp \
	&& wp --allow-root --version

# Built on the host by local-dev.sh agent-up (the "agentbin" build context).
COPY --from=agentbin --chmod=0755 jman-agent /usr/local/bin/jman-agent
COPY --chmod=0755 entrypoint.sh /usr/local/bin/local-dev-server
COPY --chmod=0755 fixtures.sh /usr/local/bin/local-dev-fixtures
COPY --chmod=0600 agent.toml /etc/jman-agent/config.toml
COPY --chmod=0644 sshd.conf /etc/ssh/sshd_config.d/local-dev.conf

ENTRYPOINT ["/usr/local/bin/local-dev-server"]
