#!/usr/bin/env bash
set -euo pipefail

app_dir=/home/user/avari-domains
stage_dir="$(mktemp -d /tmp/avari-domains-deploy.XXXXXX)"
trap 'rm -rf "$stage_dir"' EXIT

tar -xz --no-same-owner --no-same-permissions -C "$stage_dir"
test -f "$stage_dir/Dockerfile"
test -f "$stage_dir/deploy/deploy-vps.sh"
sudo install -d -o user -g user -m 755 "$app_dir"
rsync -a --exclude='.env' --exclude='data/' --exclude='*.sqlite*' \
  "$stage_dir/" "$app_dir/"
chmod +x "$app_dir/deploy/deploy-vps.sh"
cd "$app_dir"
./deploy/deploy-vps.sh
