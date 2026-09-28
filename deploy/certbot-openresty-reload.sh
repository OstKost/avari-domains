#!/bin/sh
set -eu

if [ "${RENEWED_LINEAGE:-}" = /etc/letsencrypt/live/domains.avari.dev ]; then
  /usr/bin/openresty -p /opt/om/nginx/ -t
  /usr/bin/openresty -p /opt/om/nginx/ -s reload
fi
