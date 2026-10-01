#!/usr/bin/env bash
set -euo pipefail

app_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
site_conf=/opt/om/nginx/conf/sites/8.conf
challenge_dir=/var/www/letsencrypt
domain=domains.avari.dev

cd "$app_dir"

sudo install -d -m 755 "$challenge_dir/.well-known/acme-challenge"
sudo install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
sudo install -m 755 deploy/certbot-openresty-reload.sh \
  /etc/letsencrypt/renewal-hooks/deploy/avari-domains
if sudo test -f "$site_conf" && ! sudo test -f "$site_conf.avari-domains-original"; then
  sudo cp -a "$site_conf" "$site_conf.avari-domains-original"
fi
# Issue the certificate while the HTTP challenge location is already active.
if ! sudo test -s "/etc/letsencrypt/live/$domain/fullchain.pem"; then
  sudo install -m 644 deploy/openresty-domains-acme.conf "$site_conf"
  sudo /usr/bin/openresty -p /opt/om/nginx/ -t
  sudo /usr/bin/openresty -p /opt/om/nginx/ -s reload
  sudo certbot certonly --webroot -w "$challenge_dir" -d "$domain" \
    --non-interactive --agree-tos --register-unsafely-without-email
fi

sudo install -m 644 deploy/openresty-domains.conf "$site_conf"
sudo /usr/bin/openresty -p /opt/om/nginx/ -t
sudo /usr/bin/openresty -p /opt/om/nginx/ -s reload
sudo docker compose up -d --build --remove-orphans

for attempt in {1..30}; do
  if curl --fail --silent http://127.0.0.1:18080/api/health >/dev/null; then
    break
  fi
  sleep 2
done
curl --fail --silent http://127.0.0.1:18080/api/health
