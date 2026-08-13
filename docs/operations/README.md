# AstraStoreXion 单节点运维手册

本文描述博客融合版本的生产部署。它是一个持久化、回环监听的单节点文件服务；Django 负责公网认证和业务元数据，Xion 只负责文件字节、原始名称和 SHA-256。

## 安全边界

- 仅监听 `127.0.0.1:8081`，不在 Nginx 暴露 Xion 端口。
- 使用独立的 `astrastore-xion` 系统用户，数据目录为 `/var/lib/astrastore-xion`。
- 服务密钥只存在于 mode 0600 的 `/etc/astrastore-xion.env` 和博客受限环境文件。
- 浏览器、Git、构建产物和日志中都不能出现服务密钥。
- 单文件默认上限为 50 MB。

## 安装

在已验证源码上构建 Linux 二进制：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags='-s -w' -o bin/astrastore-xion ./core/apigateway
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

数据目录结构包含 `objects/`、`metadata/` 和 `tmp/`。一致备份应在停止写入或停止服务后进行：

```bash
sudo systemctl stop astrastore-xion
sudo tar -C /var/lib -czf /var/backups/astrastore-xion-$(date +%Y%m%d-%H%M%S).tar.gz astrastore-xion
sudo systemctl start astrastore-xion
```

恢复时先停止服务，将备份解压到原路径，确认所有者为 `astrastore-xion:astrastore-xion` 和目录 mode 0700，再启动并执行 `/readyz` 与 fixture 下载校验。

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

告警优先级：就绪失败、磁盘使用率超过 80%、博客上传持续 5xx、对象目录和元数据目录数量长期不一致。

## 已知限制

当前生产形态是单节点，不提供副本、自动故障转移或跨区域容灾。旧媒体本轮不迁移。业务重要文件仍需要主机级定期备份；后续扩容前必须先设计副本一致性和迁移策略。
