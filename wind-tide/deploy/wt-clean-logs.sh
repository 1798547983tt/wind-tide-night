#!/usr/bin/env bash
# wind-tide: wt-clean-logs.sh frees disk on the server every few days
# (wind-tide/DEPLOY.md §12). It never touches the leaderboard and pool tables
# (wt_call_days, wt_call_streaks, wt_ais_*, wt_pool_events, users): only the
# relay, system and error rows of the logs table older than KEEP_DAYS go, plus
# unused Docker images and build cache older than IMAGE_DAYS, oversized
# container logs, the journal, rotated system logs and the package cache.
# Install: cp wt-clean-logs.sh /usr/local/bin/ && chmod 755 /usr/local/bin/wt-clean-logs.sh
# Cron:    30 4 */5 * * /usr/local/bin/wt-clean-logs.sh >> /var/log/wt-clean-logs.log 2>&1
set -u
SITE_DIR=${SITE_DIR:-/opt/wind-tide}
DB="$SITE_DIR/data/one-api.db"
KEEP_DAYS=${KEEP_DAYS:-15}   # logs rows younger than this stay; the week board needs 7
IMAGE_DAYS=${IMAGE_DAYS:-5}  # unused images and journal entries younger than this stay
PROTECT_TAGS="wt-sqlite:latest wind-tide-site:previous"   # backup tool, rollback image

log() { echo "$(date '+%F %T') $*"; }
disk() { df -h / | awk 'NR==2 {print $3 " used, " $4 " free (" $5 ")"}'; }
log "== wt-clean-logs start: $(disk)"

# 1. logs table (SQLite sites): consume (2), system (4) and error (5) rows older
#    than KEEP_DAYS, in batches so the site never waits long for the write lock.
if [ -f "$DB" ] && command -v sqlite3 >/dev/null 2>&1; then
  cutoff=$(( $(date +%s) - KEEP_DAYS * 86400 ))
  total=0
  while :; do
    n=$(printf '.timeout 15000\nDELETE FROM logs WHERE id IN (SELECT id FROM logs WHERE created_at < %s AND type IN (2,4,5) ORDER BY id LIMIT 2000);\nSELECT changes();\n' "$cutoff" | sqlite3 "$DB" 2>&1 | tail -1)
    case "$n" in ''|*[!0-9]*) log "logs: sqlite said: $n"; break;; esac
    total=$(( total + n ))
    [ "$n" -ge 2000 ] || break
    sleep 1
  done
  printf '.timeout 15000\nPRAGMA wal_checkpoint(TRUNCATE);\n' | sqlite3 "$DB" >/dev/null 2>&1
  log "logs: deleted $total rows older than $KEEP_DAYS days"
else
  log "logs: skipped ($DB missing or no sqlite3 on this host)"
fi

# 2. Docker: images no container uses, older than IMAGE_DAYS, except the protected tags.
protect=" "
for c in $(docker ps -aq); do protect="$protect$(docker inspect --format '{{.Image}}' "$c") "; done
for t in $PROTECT_TAGS; do
  id=$(docker images -q --no-trunc "$t" 2>/dev/null | head -1)
  [ -n "$id" ] && protect="$protect$id "
done
cut_img=$(( $(date +%s) - IMAGE_DAYS * 86400 ))
removed=0
for id in $(docker images -q --no-trunc | sort -u); do
  case "$protect" in *" $id "*) continue;; esac
  created=$(docker inspect --format '{{.Created}}' "$id" 2>/dev/null) || continue
  ts=$(date -d "$created" +%s 2>/dev/null) || continue
  [ "$ts" -lt "$cut_img" ] || continue
  docker rmi -f "$id" >/dev/null 2>&1 && removed=$(( removed + 1 ))
done
docker builder prune -af --filter "until=$(( IMAGE_DAYS * 24 ))h" >/dev/null 2>&1
log "docker: removed $removed unused image(s) older than $IMAGE_DAYS days"

# 3. Container logs over 20 MB are emptied in place (the json-file driver keeps appending).
find /var/lib/docker/containers -name '*-json.log' -size +20M -exec truncate -s 0 {} + 2>/dev/null

# 4. Journal, rotated system logs and the package cache.
journalctl --vacuum-time="${IMAGE_DAYS}d" --vacuum-size=100M >/dev/null 2>&1
find /var/log -maxdepth 1 -type f -name '*.gz' -mtime +"$IMAGE_DAYS" -delete 2>/dev/null
find /var/log -maxdepth 1 -type f -name '*-20[0-9][0-9][0-9][0-9][0-9][0-9]' -mtime +"$IMAGE_DAYS" -delete 2>/dev/null
find /var/log -maxdepth 1 -type f -name '*.[0-9]' -mtime +"$IMAGE_DAYS" -delete 2>/dev/null
if command -v dnf >/dev/null 2>&1; then dnf clean all -q >/dev/null 2>&1
elif command -v yum >/dev/null 2>&1; then yum clean all -q >/dev/null 2>&1
elif command -v apt-get >/dev/null 2>&1; then apt-get clean >/dev/null 2>&1
fi
log "== wt-clean-logs done: $(disk)"
