#!/bin/sh
set -eu

backup_dir=/var/lib/fluxeando/backups/mysql
password_file=/run/secrets/mysql_app_password

case "${BACKUP_TIME:-}" in
  [0-2][0-9]:[0-5][0-9]) ;;
  *) echo "BACKUP_TIME must use HH:MM" >&2; exit 1 ;;
esac
case "${BACKUP_RETENTION_DAYS:-}" in
  ''|*[!0-9]*) echo "BACKUP_RETENTION_DAYS must be a positive integer" >&2; exit 1 ;;
esac

mkdir -p "$backup_dir"
chmod 0700 "$backup_dir"

last_run=""
while :; do
  today="$(date +%F)"
  now="$(date +%H:%M)"
  if [ "$now" = "$BACKUP_TIME" ] && [ "$last_run" != "$today" ]; then
    temporary="$backup_dir/.${MYSQL_DATABASE}-${today}.sql.gz.tmp"
    output="$backup_dir/${MYSQL_DATABASE}-${today}.sql.gz"
    export MYSQL_PWD="$(cat "$password_file")"
    if mysqldump --host="$MYSQL_HOST" --user="$MYSQL_USER" --single-transaction --routines --events --triggers --databases "$MYSQL_DATABASE" | gzip -c > "$temporary"; then
      mv "$temporary" "$output"
      find "$backup_dir" -type f -name '*.sql.gz' -mtime "+$BACKUP_RETENTION_DAYS" -delete
      last_run="$today"
      echo "MySQL backup completed: $output"
    else
      rm -f "$temporary"
      echo "MySQL backup failed" >&2
    fi
    unset MYSQL_PWD
  fi
  sleep 30
done
