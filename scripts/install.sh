#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run this script as root" >&2
  exit 1
fi

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${1:-$repo_dir/dist/cute-mafia-api}"
config="${2:-$repo_dir/backend/config.yaml}"
test -f "$binary" || { echo "Binary not found: $binary" >&2; exit 1; }
test -f "$config" || { echo "Config not found: $config" >&2; exit 1; }

read -r -p "Let's Encrypt email: " cert_email
read -r -p "GitHub Actions deploy public key (ssh-ed25519 ...): " deploy_key
test -n "$cert_email" && test -n "$deploy_key"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y nginx certbot python3-certbot-nginx curl sudo

id cute-mafia >/dev/null 2>&1 || useradd --system --home /var/lib/cute-mafia --shell /usr/sbin/nologin cute-mafia
install -d -o cute-mafia -g cute-mafia -m 0750 /var/lib/cute-mafia /etc/cute-mafia
install -o root -g root -m 0755 "$binary" /usr/local/bin/cute-mafia-api
install -o cute-mafia -g cute-mafia -m 0600 "$config" /etc/cute-mafia/config.yaml

cat >/etc/systemd/system/cute-mafia-api.service <<'UNIT'
[Unit]
Description=Cute Mafia API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cute-mafia
Group=cute-mafia
ExecStart=/usr/local/bin/cute-mafia-api -config /etc/cute-mafia/config.yaml
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/cute-mafia

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/nginx/conf.d/cute-mafia-rate-limit.conf <<'NGINX'
limit_req_zone $binary_remote_addr zone=cute_mafia_api:10m rate=10r/s;
NGINX

cat >/etc/nginx/sites-available/cute-mafia-api <<'NGINX'
server {
    listen 80;
    listen [::]:80;
    server_name api.cute-mafia.ru;

    client_max_body_size 101M;
    gzip on;
    gzip_types application/json application/javascript text/css text/plain image/svg+xml;

    location /api/ {
        limit_req zone=cute_mafia_api burst=30 nodelay;
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }
}
NGINX
ln -sfn /etc/nginx/sites-available/cute-mafia-api /etc/nginx/sites-enabled/cute-mafia-api
rm -f /etc/nginx/sites-enabled/default

id deploy >/dev/null 2>&1 || useradd --create-home --shell /bin/bash deploy
install -d -o deploy -g deploy -m 0700 /home/deploy/.ssh
printf '%s\n' "$deploy_key" >/home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 0600 /home/deploy/.ssh/authorized_keys
install -o root -g root -m 0755 "$repo_dir/scripts/deploy-cute-mafia" /usr/local/sbin/deploy-cute-mafia
printf '%s\n' 'deploy ALL=(root) NOPASSWD: /usr/local/sbin/deploy-cute-mafia' >/etc/sudoers.d/cute-mafia-deploy
chmod 0440 /etc/sudoers.d/cute-mafia-deploy
visudo -cf /etc/sudoers.d/cute-mafia-deploy

systemctl daemon-reload
systemctl enable --now cute-mafia-api
nginx -t
systemctl reload nginx
certbot --nginx --non-interactive --agree-tos --redirect --email "$cert_email" -d api.cute-mafia.ru

echo "Installed. Check: systemctl status cute-mafia-api"
