#!/usr/bin/env bash
#
# 备份博客数据库、历史 media 目录和 AstraStoreXion 对象目录。
#
# Xion 用「临时文件 → fsync → rename」保证单次写入的原子性，但一次普通备份
# 仍可能同时看到某个对象的旧 manifest 和新字节。因此这里在打包前停止
# astrastore-xion，打包后无论成功与否都恢复服务。
#
# 用法：
#   sudo ./deploy/backup/astrastore-xion-backup.sh
#
# 可用环境变量覆盖：
#   BACKUP_DIR            默认 /root/backups
#   BACKUP_RETENTION_DAYS 默认 7
#   XION_DATA_DIR         默认 /var/lib/astrastore-xion
#   BLOG_ENV_FILE         默认 /opt/blog_li/.env
#   BLOG_MEDIA_DIR        默认 /opt/blog_li/media
#   XION_SERVICE          默认 astrastore-xion
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/root/backups}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
XION_DATA_DIR="${XION_DATA_DIR:-/var/lib/astrastore-xion}"
BLOG_ENV_FILE="${BLOG_ENV_FILE:-/opt/blog_li/.env}"
BLOG_MEDIA_DIR="${BLOG_MEDIA_DIR:-/opt/blog_li/media}"
XION_SERVICE="${XION_SERVICE:-astrastore-xion}"

STAMP="$(date +%F)"
STARTED_AT="$(date +%s)"
XION_STOPPED=0

# 单实例锁：备份会停写服务，绝不能让两次运行叠加。
exec 9>/run/astrastore-xion-backup.lock
if ! flock -n 9; then
  printf '%s another backup is already running; exiting\n' "$(date -Is)" >&2
  exit 0
fi

log() {
  printf '%s %s\n' "$(date -Is)" "$*"
}

fail() {
  printf '%s ERROR: %s\n' "$(date -Is)" "$*" >&2
  exit 1
}

restore_service() {
  if [ "$XION_STOPPED" -eq 1 ]; then
    log "starting ${XION_SERVICE}"
    systemctl start "$XION_SERVICE" || log "failed to start ${XION_SERVICE}; it needs manual attention"
    XION_STOPPED=0
  fi
}

# 即使是打包中途失败，也要把服务放回去。
trap restore_service EXIT

mkdir -p "$BACKUP_DIR"

# ---------------------------------------------------------------------------
# 1. MySQL
# ---------------------------------------------------------------------------
if [ -f "$BLOG_ENV_FILE" ]; then
  log "dumping MySQL"
  set -a
  # shellcheck disable=SC1090
  source "$BLOG_ENV_FILE"
  set +a
  MYSQL_PWD="${DB_PASSWORD:?DB_PASSWORD missing from ${BLOG_ENV_FILE}}" mysqldump \
    --single-transaction \
    --quick \
    -h"${DB_HOST:-localhost}" \
    -P"${DB_PORT:-3306}" \
    -u"${DB_USER:?DB_USER missing from ${BLOG_ENV_FILE}}" \
    "${DB_NAME:?DB_NAME missing from ${BLOG_ENV_FILE}}" \
    | gzip > "$BACKUP_DIR/blog-$STAMP.sql.gz"
else
  log "skipping MySQL: ${BLOG_ENV_FILE} not found"
fi

# ---------------------------------------------------------------------------
# 2. 历史 media 目录
# ---------------------------------------------------------------------------
if [ -d "$BLOG_MEDIA_DIR" ]; then
  log "syncing ${BLOG_MEDIA_DIR}"
  mkdir -p "$BACKUP_DIR/media"
  rsync -a --delete "$BLOG_MEDIA_DIR/" "$BACKUP_DIR/media/"
else
  log "skipping media: ${BLOG_MEDIA_DIR} not found"
fi

# ---------------------------------------------------------------------------
# 3. AstraStoreXion 对象目录
# ---------------------------------------------------------------------------
if [ -d "$XION_DATA_DIR" ]; then
  log "stopping ${XION_SERVICE} for a consistent snapshot"
  systemctl stop "$XION_SERVICE"
  XION_STOPPED=1

  log "archiving ${XION_DATA_DIR}"
  tar -C "$(dirname "$XION_DATA_DIR")" -czf "$BACKUP_DIR/xion-$STAMP.tar.gz" "$(basename "$XION_DATA_DIR")"

  restore_service

  # 归档必须可读，且记录校验和，恢复时用来判断文件是否损坏。
  [ -s "$BACKUP_DIR/xion-$STAMP.tar.gz" ] || fail "xion archive is empty"
  sha256sum "$BACKUP_DIR/xion-$STAMP.tar.gz" > "$BACKUP_DIR/xion-$STAMP.tar.gz.sha256"
  tar -tzf "$BACKUP_DIR/xion-$STAMP.tar.gz" >/dev/null || fail "xion archive is unreadable"
else
  log "skipping Xion: ${XION_DATA_DIR} not found"
fi

# ---------------------------------------------------------------------------
# 4. 保留期
# ---------------------------------------------------------------------------
log "pruning backups older than ${RETENTION_DAYS} days"
find "$BACKUP_DIR" -maxdepth 1 -name 'blog-*.sql.gz' -mtime +"$RETENTION_DAYS" -delete
find "$BACKUP_DIR" -maxdepth 1 -name 'xion-*.tar.gz' -mtime +"$RETENTION_DAYS" -delete
find "$BACKUP_DIR" -maxdepth 1 -name 'xion-*.tar.gz.sha256' -mtime +"$RETENTION_DAYS" -delete

ELAPSED=$(( $(date +%s) - STARTED_AT ))
log "backup finished in ${ELAPSED}s"
ls -lh "$BACKUP_DIR"/blog-"$STAMP".sql.gz "$BACKUP_DIR"/xion-"$STAMP".tar.gz 2>/dev/null || true
