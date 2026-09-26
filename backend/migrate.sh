#!/bin/sh
set -eu

# Fail immediately if this container has no database connection setting.
: "${DATABASE_URL:?DATABASE_URL must be set}"

# Replace this shell with the migration runner.
# "$@" forwards arguments such as "up", "version", or "force 1".
exec migrate \
  -path /migrations \
  -database "$DATABASE_URL" \
  "$@"
