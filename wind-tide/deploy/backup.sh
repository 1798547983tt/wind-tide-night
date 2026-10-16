#!/bin/sh
# wind-tide: daily backup of the site database, keeping the last seven days.
# Cron runs it on the server; it uses the small wt-sqlite image built during
# setup (wind-tide/DEPLOY.md), whose SQLite is newer than the host's.
set -eu
# The copies hold relay keys and the Discord client secret: only root may
# read them.
umask 077
cd "$(dirname "$0")"
mkdir -p backups
chmod 700 backups
stamp=$(date +%Y%m%d)
docker run --rm -v "$PWD/data:/data" -v "$PWD/backups:/backups" wt-sqlite \
	sqlite3 /data/one-api.db ".backup '/backups/one-api-$stamp.db'"
gzip -f "backups/one-api-$stamp.db"
# The copy is written inside the container, outside this script's umask.
chmod 600 "backups/one-api-$stamp.db.gz"
find backups -name 'one-api-*.db.gz' -mtime +6 -delete
