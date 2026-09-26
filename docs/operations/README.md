# AstraStoreXion 单节点运维手册

本文描述博客融合版本的生产部署。它是一个持久化、回环监听的单节点文件服务；Django 负责公网认证和业务元数据，Xion 只负责文件字节、原始名称和 SHA-256。

## 安全边界

- 仅监听 `127.0.0.1:8081`，不在 Nginx 暴露 Xion 端口。
- 使用独立的 `astrastore-xion` 系统用户，数据目录为 `/var/lib/astrastore-xion`。
- 服务密钥只存在于 mode 0600 的 `/etc/astrastore-xion.env` 和博客受限环境文件。
- 浏览器、Git、构建产物和日志中都不能出现服务密钥。
- 单文件默认上限为 1 GiB。

## 安装

在已验证源码上构建 Linux 二进制：

```bash
make release-linux
ls -l bin/linux-amd64
```

服务器上创建用户并安装：

```bash
sudo useradd --system --home-dir /var/lib/astrastore-xion \
  --shell /usr/sbin/nologin astrastore-xion
sudo install -o root -g root -m 0755 bin/astrastore-xion /usr/local/bin/astrastore-xion
sudo install -o root -g root -m 0644 \
  deploy/systemd/astrastore-xion.service \
  /etc/systemd/system/astrastore-xion.service
```

直接在服务器生成密钥，不把值带回开发机：

```bash
sudo install -o root -g root -m 0600 \
  deploy/systemd/astrastore-xion.env.example \
  /etc/astrastore-xion.env
printf 'XION_SERVICE_TOKEN=%s\n' "$(openssl rand -hex 32)" | \
  sudo tee -a /etc/astrastore-xion.env >/dev/null
```

启动并验证：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now astrastore-xion
systemctl is-active astrastore-xion
curl --fail http://127.0.0.1:8081/healthz
curl --fail http://127.0.0.1:8081/readyz
ss -ltnp | grep '127.0.0.1:8081'
```

安装服务器命令行工具：

```bash
make xionctl
sudo XIONCTL_BINARY="$PWD/bin/xionctl" ./deploy/xionctl-install.sh
xionctl health
xionctl capacity
xionctl config
xionctl config --max-upload-size 100M
xionctl list | jq '.results[] | {id: .file_id, name: .filename, size}'
```

日常文件操作：

```bash
xionctl info <file-id>
xionctl upload /path/to/file.png
xionctl download <file-id> /tmp/file.png
xionctl delete <file-id> --yes
xionctl logs --lines 100
xionctl logs --since 1h
xionctl logs --follow
```

`xionctl capacity` 使用类似 Linux `df -h` 的终端表格查看总量、已用、可用、使用率和上传状态。环境模板默认配置 `XION_STORAGE_PAUSE_AT_PERCENT=90`：达到阈值后 Xion 自动拒绝新上传并返回 HTTP 507，现有文件仍可读取/下载，删除文件释放空间后下一次上传自动恢复。该机制不迁移文件到其他节点；跨节点复制需要单独的复制协议。

`xionctl config` 查看和修改 Xion 上传限制。修改前自动在同目录生成带时间戳的 `.backup-YYYYMMDD-HHMMSS` 配置备份，使用临时文件原子替换，然后重启 `astrastore-xion.service`：

```bash
xionctl config
xionctl config --max-upload-size 100M
```

支持 `K`、`M`、`G`、`T` 后缀。该限制只作用于 Xion；博客 Django 仍有独立的文件大小校验，扩大线上上传上限时必须同步调整博客配置和前端限制。

`xionctl` 通过 Xion HTTP API 工作，绝不直接删除对象目录；删除前必须确认文件 ID，命令也要求显式提供 `--yes`。服务令牌只从 `/etc/astrastore-xion.env` 读取，不会打印到终端。

`logs` 只读取固定单元 `astrastore-xion.service` 的 journal。默认最近 100 行；`--since 1h` 查看最近一小时，`--follow` 持续跟踪。它不经过 shell，不允许指定其他 systemd 服务，也不会改变 Xion 运行状态。

## 存储修复与容量回收

### 启动自愈

服务启动时会先执行一次修复，把崩溃残留的临时文件移入 `quarantine/`（对象字节不会被删除，只是离开活动目录），然后才接受流量。这一步解决了"进程在写 manifest 与改名之间被杀、`/readyz` 永久报错"的问题：

```bash
journalctl -u astrastore-xion --since '10 min ago' | grep recovered
# recovered storage: partial_objects=1 metadata_manifests=2 ...
```

`quarantine/` 只允许 `partials/`、`metadata/`、`uploads/` 三个子目录，出现其他内容时 `/readyz` 会重新报警，避免"把问题藏起来"。确认无用后可人工清理：

```bash
sudo ls -R /var/lib/astrastore-xion/quarantine
sudo rm -rf /var/lib/astrastore-xion/quarantine/*
```

### 清理未完成的分片会话

分片会话按声明的完整大小占用 owner 配额。客户端中断后如果不清理，配额会被一直占住，因此服务按 `XION_UPLOAD_SESSION_TTL` 自动过期（模板默认 `24h`），也可以手动触发：

```bash
xionctl uploads list
xionctl uploads purge
```

### 回收站出口

普通删除只把对象移入回收站，不释放空间。回收站不会自动清理，需要显式操作：

```bash
xionctl trash list --limit 100
xionctl trash restore <file-id>
xionctl trash purge --older-than 720h --yes
xionctl trash purge --all --yes
```

`--older-than` 接受 Go duration（`24h`、`720h`），也接受 RFC3339 时间戳。永久删除必须提供 `--yes`，且必须给出 `--older-than` 或 `--all`，避免"无参数清空回收站"这类误操作。

### 版本核对

发布二进制注入版本与 commit，出问题时先确认线上到底是哪一次构建：

```bash
xionctl version
# xionctl v0.4.0 (a1b2c3d)
```

## 博客启用顺序

1. 备份 MySQL、`/opt/blog_li` 代码、`.env` 和当前前端 release 指向。
2. 安装并启动 Xion，验证回环监听和就绪检查。
3. 部署 Django 代码但保持 `XION_STORAGE_ENABLED=false`。
4. 安装 Python SDK、执行 `manage.py check` 和迁移。
5. 原子切换 Vue 静态 release，验证登录和旧 `/media/` 链接。
6. 在博客受限环境文件写入 Xion URL 和同一服务密钥，再将开关设为 `true`。
7. 重启博客服务，用专用测试文件做上传、下载、重启后读取和删除验收。

Django 环境项：

```dotenv
XION_STORAGE_ENABLED=true
XION_BASE_URL=http://127.0.0.1:8081
XION_SERVICE_TOKEN=<same server-side secret>
XION_CONNECT_TIMEOUT=3
XION_READ_TIMEOUT=300
XION_MAX_RETRIES=2
```

## 上线验收

对 `test/fixtures/generated/` 中 PNG、PDF、DOCX、TXT 分别运行：

```bash
XION_BASE_URL=http://127.0.0.1:8081 \
XION_SERVICE_TOKEN="$XION_SERVICE_TOKEN" \
./scripts/smoke-test.sh test/fixtures/generated/storage-test-cover.png
```

验收还必须覆盖：

- 通过博客 API 上传后，数据库的 `storage_backend` 为 `xion`。
- 下载 SHA-256 与 `manifest.json` 一致。
- 重启 Xion 和博客进程后仍可下载。
- 删除专用测试记录后，Xion 对象同步消失。
- 至少抽查一个历史 `/media/` URL 返回 200。

## 备份与恢复

数据目录结构包含 `objects/`、`metadata/`、`tmp/`、`uploads/`、`trash/` 和 `quarantine/`。

### 自动备份

`deploy/backup/` 提供一套 systemd 单元，每天 04:30 备份数据库、历史 media 与 Xion 对象：

```bash
sudo install -o root -g root -m 0755 deploy/backup/astrastore-xion-backup.sh /usr/local/bin/astrastore-xion-backup
sudo install -o root -g root -m 0644 deploy/backup/astrastore-xion-backup.service /etc/systemd/system/
sudo install -o root -g root -m 0644 deploy/backup/astrastore-xion-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now astrastore-xion-backup.timer
```

脚本在打包 Xion 目录之前会**停止 astrastore-xion**，无论成功失败都由 `trap` 恢复服务。原因是一次普通备份可能同时读到某个对象的旧 manifest 和新字节；单次写入的原子性并不保证目录整体一致。归档后用 `sha256sum` 记录校验和并试读 tar，确认不是半个文件。

```bash
sudo systemctl start astrastore-xion-backup.service   # 手动跑一次
sudo systemctl list-timers astrastore-xion-backup.timer
sudo journalctl -u astrastore-xion-backup --since '1 day ago'
sudo systemctl --failed                              # 失败会进入 failed 状态，必须能看到
```

保留期默认 7 天（`BACKUP_RETENTION_DAYS`）。产物位于 `/root/backups`：`blog-YYYY-MM-DD.sql.gz`、`xion-YYYY-MM-DD.tar.gz` 及其 `.sha256`、`media/` 增量副本。

> **同盘风险。** 默认 `/root/backups` 与数据目录都在同一块盘上，只能防误删和一致性损坏，**不能防磁盘故障或整机丢失**。生产上必须把 `/root/backups` 再同步到异地，例如：
>
> ```bash
> rsync -a --delete /root/backups/ user@offsite-host:/backups/$(hostname)/
> ```
>
> 这一步没有内置进脚本，因为异地目标与凭据取决于你的环境；如果长期不做，等于只有半套备份。

### 恢复演练

备份的唯一价值是能恢复，所以恢复步骤必须实测过。下面是可重复的演练流程，**不会碰线上数据目录**：

```bash
drill="$(mktemp -d)"
tar -xzf /root/backups/xion-YYYY-MM-DD.tar.gz -C "$drill"
sha256sum -c /root/backups/xion-YYYY-MM-DD.tar.gz.sha256

# 用备份数据起一个临时实例，验证它能自行通过一致性自检
XION_SERVICE_TOKEN=drill XION_DATA_DIR="$drill/astrastore-xion" \
  XION_LISTEN_ADDR=127.0.0.1:18099 /usr/local/bin/astrastore-xion &
curl -fsS http://127.0.0.1:18099/readyz

# 逐个比对 manifest 记录的 checksum 与实际字节
for m in "$drill"/astrastore-xion/metadata/*.json; do
  id="$(basename "$m" .json)"
  want="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["checksum"])' "$m")"
  got="$(sha256sum "$drill/astrastore-xion/objects/$id" | awk '{print $1}')"
  [ "$want" = "$got" ] || echo "MISMATCH: $id"
done
```

真正的恢复：停止服务 → 把数据目录整体替换为备份内容 → 确认属主为 `astrastore-xion:astrastore-xion`、目录 mode 0700 → 启动服务 → 执行 `/readyz` 与 fixture 下载校验。**先保留旧目录**，确认新数据可用后再删除。

## 回滚

按以下顺序回滚，避免产生新对象：

1. 将博客 `XION_STORAGE_ENABLED=false`，重启博客，停止新写入 Xion。
2. 将前端 `current` 指回上一 release，并 reload Nginx。
3. 若数据库中还没有 `storage_backend=xion` 的记录，可恢复适配器引入前的后端代码；一旦已有 Xion 记录，必须保留包含双存储适配器的兼容后端版本，不能直接恢复纯本地旧代码。
4. 保留 `/var/lib/astrastore-xion`，不要删除已有对象；已有 `storage_backend=xion` 的记录始终需要兼容适配器与 Xion 服务读取。
5. 如确需回到纯本地旧后端，必须先把全部 Xion 对象迁回 `media/`、校验 SHA-256 并在事务中更新记录，完成数据库与对象备份后才能切换。
6. 修复完成后先启服务、验证 `/readyz`，再启博客写入开关。

## 日常检查

```bash
systemctl is-active astrastore-xion blog-li
journalctl -u astrastore-xion --since '1 hour ago' --no-pager
curl --fail http://127.0.0.1:8081/readyz
du -sh /var/lib/astrastore-xion
df -h /var/lib/astrastore-xion
xionctl capacity
```

告警优先级：就绪失败、磁盘使用率超过 80%、博客上传持续 5xx、对象目录和元数据目录数量长期不一致。容量与数量统计直接读自服务内存索引，不再依赖逐目录扫描；`xionctl capacity` 的 `object_count` 与实际对象数不一致时，说明有人绕过服务改动过数据目录，需要重启服务或调用重建。

## 已知限制

当前生产形态是单节点，不提供副本、自动故障转移或跨区域容灾。旧媒体本轮不迁移。业务重要文件仍需要主机级定期备份；后续扩容前必须先设计副本一致性和迁移策略。
