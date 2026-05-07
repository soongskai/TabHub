#!/bin/sh
set -eu

data_dir="${DATA_DIR:-/app/data}"

mkdir -p "$data_dir/uploads/wallpapers" "$data_dir/uploads/icons"

if [ "$(id -u)" = "0" ]; then
  chown -R appuser:appuser "$data_dir"
  exec su-exec appuser "$@"
fi

exec "$@"
