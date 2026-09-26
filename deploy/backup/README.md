# 主机备份

`astrastore-xion-backup.{service,timer}` 每天 04:30 备份三样东西：

| 产物 | 内容 |
| --- | --- |
| `blog-YYYY-MM-DD.sql.gz` | MySQL 业务库（`/opt/blog_li/.env` 里的连接信息） |
| `media/` | 历史 `/opt/blog_li/media` 增量副本 |
| `xion-YYYY-MM-DD.tar.gz` + `.sha256` | AstraStoreXion 数据目录整体归档 |

默认写入 `/root/backups`，保留 7 天（`BACKUP_RETENTION_DAYS`）。

## 安装

```bash
sudo install -o root -g root -m 0755 deploy/backup/astrastore-xion-backup.sh /usr/local/bin/astrastore-xion-backup
sudo install -o root -g root -m 0644 deploy/backup/astrastore-xion-backup.service /etc/systemd/system/
sudo install -o root -g root -m 0644 deploy/backup/astrastore-xion-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now astrastore-xion-backup.timer
sudo systemctl start astrastore-xion-backup.service   # 立即验证一次
```

`deploy/backup/` 会被发布流水线同步到服务器，所以单元内容不会随重建服务器而丢失。

## 为什么要停服务

单次写入是原子的（临时文件 → fsync → 原子改名），但**目录整体不是**：普通
`tar` 可能读到某个对象的新字节配旧 manifest。脚本因此先 `systemctl stop
astrastore-xion`，并用 `trap` 保证无论成功失败都恢复服务。归档后会
`sha256sum` 并试读 tar，半个文件不会被当成有效备份。

## 异地副本（必须补上）

`/root/backups` 与数据目录在同一块盘上，只能防误删和一致性损坏，**防不了
磁盘故障或整机丢失**。脚本没有内置异地传输，因为目标与凭据取决于你的环境，
但会每天在 journal 里明确告警：

```
WARNING: no OFFSITE_DIR configured; backups exist only on this host
```

接上异地目标后告警消失。两种常见做法：

**A. 已挂载的异地目录（NAS 等）**

```bash
sudo install -d /mnt/nas/xion-backups
sudo systemctl edit astrastore-xion-backup.service
# 加入：
# [Service]
# Environment=OFFSITE_DIR=/mnt/nas/xion-backups
```

**B. 独立主机 rsync**

```bash
# 在备份机上生成专用密钥，authorized_keys 建议限制为只允许写入该目录
rsync -a --delete /root/backups/ backup@offsite-host:/backups/$(hostname)/
```

把这条命令放进一个 `OnCalendar=daily` 的 timer 或备份脚本的 `OFFSITE_DIR`
路径中。**没有异地副本的备份只能算半套**，请择一落地。

## 失败可见性

这个单元曾经因为脚本从 macOS 拷贝时缺少执行位（`203/EXEC`）静默失败了数周。
现在请把下面这条纳入日常检查：

```bash
systemctl --failed
systemctl list-timers astrastore-xion-backup.timer
sudo journalctl -u astrastore-xion-backup --since '2 days ago'
```

## 恢复

见 [单节点运维手册](../../docs/operations/README.md) 的"备份与恢复"一节，
其中包含一个在临时实例上运行、不触碰线上数据的恢复演练流程。
