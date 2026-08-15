# AstraStoreXion

AstraStoreXion 是一个面向自托管博客的持久化文件服务。当前生产可用形态为单节点：Go HTTP 服务负责文件字节、原始名称与 SHA-256，Django 负责公网认证和业务元数据，Vue 管理端提供上传进度和内置教程。

## 当前能力

- 原子上传：对象与 JSON manifest 分离落盘，失败自动补偿。
- 重启持久化：服务重启后可继续查询和下载。
- 安全 ID：拒绝路径穿越和非法对象 ID。
- 完整生命周期：上传、列表、状态、下载、幂等删除。
- 服务端认证：常量时间比较 Bearer 服务密钥。
- 1 GiB 默认限制和结构化错误响应。
- Python SDK 1.1：流式下载、安全重试和结构化异常。
- systemd 最小权限部署与字节级 smoke test。
- `xionctl` 服务器命令：健康检查、列表、详情、上传、下载和安全删除。
- `xionctl logs` 查看 Xion systemd 节点运行日志。
- 容量保护：按文件系统使用率自动暂停新上传，空间释放后自动恢复；`xionctl capacity` 可查看容量和暂停状态。
- 博客双存储：新文件走 Xion，历史 `/media/` 保持可用。

仓库中的 Raft、元数据服务、存储节点和其他语言客户端仍属于实验性扩展，不是本轮博客部署的生产依赖。

## 快速开始

要求 Go 1.23+、Python 3.9+、curl。

```bash
go test ./...
make xion-service

export XION_SERVICE_TOKEN="$(openssl rand -hex 32)"
export XION_LISTEN_ADDR=127.0.0.1:8081
export XION_DATA_DIR="$(mktemp -d)"
./bin/astrastore-xion
```

在另一个终端运行：

```bash
curl --fail http://127.0.0.1:8081/readyz

XION_BASE_URL=http://127.0.0.1:8081 \
XION_SERVICE_TOKEN="$XION_SERVICE_TOKEN" \
./scripts/smoke-test.sh test/fixtures/generated/upload-smoke.txt
```

## Python SDK

```bash
python -m pip install ./client/python
```

```python
import os
from astrastore_xion import XionClient, XionConfig

config = XionConfig(
    api_gateway="http://127.0.0.1:8081",
    service_token=os.environ["XION_SERVICE_TOKEN"],
)

with XionClient(config) as client:
    with open("example.txt", "rb") as source:
        stored = client.upload_file(source, "example.txt")
    with open("downloaded.txt", "wb") as target:
        client.download_file(stored.file_id, target)
    client.delete_file(stored.file_id)
```

完整说明见[客户端使用教程](docs/客户端使用文档.md)。

服务器上直接操作 Xion：

```bash
make xionctl
sudo XIONCTL_BINARY="$PWD/bin/xionctl" ./deploy/xionctl-install.sh
xionctl health
xionctl capacity
xionctl config
xionctl config --max-upload-size 100M
xionctl list
xionctl info <file-id>
xionctl download <file-id> /tmp/file.bin
xionctl delete <file-id> --yes
xionctl logs --lines 100
xionctl logs --since 1h
xionctl logs --follow
```

`xionctl` 默认读取 `/etc/astrastore-xion.env`，不会直接修改 `/var/lib/astrastore-xion`；删除必须显式带 `--yes`。

生产环境默认 `XION_STORAGE_PAUSE_AT_PERCENT=90`。`xionctl capacity` 使用类似 Linux `df -h` 的表格显示总容量、已用、可用、使用率和上传状态。达到阈值时上传接口返回 HTTP 507 和 `storage_paused`，客户端会提示“存储空间已达到安全阈值，暂时停止上传”；读取、下载和删除仍可用，删除文件释放空间后下一次上传自动恢复，无需重启。

`xionctl config` 查看最大上传限制；使用 `xionctl config --max-upload-size 100M` 可修改限制并自动重启 Xion。命令会备份环境文件，支持 `K`、`M`、`G` 等单位；博客自身的文件类型/大小校验仍需同步调整。

普通删除现在进入回收站，不会立即释放文件数据。活动文件列表和下载不会显示回收站内容；恢复后原文件 ID 和链接保持不变：

```bash
# 查看回收站
curl -sS "$API/api/v1/trash?limit=100&offset=0" \
  -H "Authorization: Bearer $TOKEN"

# 恢复文件
curl -sS -X POST "$API/api/v1/files/<file-id>/restore" \
  -H "Authorization: Bearer $TOKEN"
```

回收站目前没有公开永久删除接口，避免误操作；后续会增加带保留期和管理员确认的清理任务。

### 可选主从异步复制

复制默认关闭。需要副节点时，在主节点环境文件中设置：

```bash
XION_REPLICA_URL=http://replica-node:8081
XION_REPLICA_TOKEN=<replica-service-token>
XION_REPLICATION_DIR=/var/lib/astrastore-xion/replication
XION_REPLICATION_MAX_ATTEMPTS=10
```

主节点上传、软删除和恢复成功后会写入持久化任务；后台任务会流式同步文件，失败时自动重试。副节点请求带有 `X-Xion-Replication: true` 标记，不会再次产生复制任务。查看状态和手动重试：

```bash
curl -sS "$API/api/v1/replication" -H "Authorization: Bearer $TOKEN"
curl -sS -X POST "$API/api/v1/replication/<job-id>/retry" \
  -H "Authorization: Bearer $TOKEN"
```

复制是异步的，不会回滚主节点写入；切换前应确认 `pending`、`running` 和 `failed` 任务已清空或已评估复制延迟。

### 可恢复分片上传

1 GiB 大文件可以使用上传会话，网络中断后从 `received_bytes` 继续，不需要重新上传：

```bash
API=http://127.0.0.1:8081
TOKEN="$XION_SERVICE_TOKEN"

# 1. 创建会话
curl -sS -X POST "$API/api/v1/uploads" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"filename":"large.zip","content_type":"application/zip","size":104857600}'

# 2. 按返回的 upload_id 和 received_bytes 顺序上传分片
curl -sS -X PUT "$API/api/v1/uploads/<upload_id>" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Range: bytes 0-1048575/104857600' \
  --data-binary @chunk-0001

# 3. 查询断点
curl -sS "$API/api/v1/uploads/<upload_id>" -H "Authorization: Bearer $TOKEN"

# 4. 所有字节上传完成后提交
curl -sS -X POST "$API/api/v1/uploads/<upload_id>/complete" \
  -H "Authorization: Bearer $TOKEN"
```

分片必须连续提交，`Content-Length` 必须等于 `Content-Range` 的分片长度；服务会重新计算完整 SHA-256。旧的 `POST /api/v1/files` 单次上传接口仍然可用。中止未完成会话使用 `DELETE /api/v1/uploads/<upload_id>`。

## 生产部署

生产服务只应监听 `127.0.0.1`，由 Django 代理业务文件操作。不要在 Nginx 暴露 8081，也不要把服务密钥交给浏览器。

- systemd 单元：`deploy/systemd/astrastore-xion.service`
- 非敏感环境模板：`deploy/systemd/astrastore-xion.env.example`
- 运维、备份、启用和回滚：[单节点运维手册](docs/operations/README.md)
- 融合架构：[设计说明](docs/superpowers/specs/2026-08-09-blog-storage-integration-design.md)

## 测试资料

`make fixtures` 会生成可重复的 PNG、PDF、DOCX、TXT 与 checksum manifest。首次生成前安装专用依赖；它们不属于服务运行时依赖。PDF 和 Word 文件已按渲染结果做视觉验证，用于本地与生产上传/下载验收。

```bash
python -m pip install -r scripts/requirements-artifacts.txt
make fixtures
ls -lh test/fixtures/generated
```

## 验证

```bash
go test ./... -count=1
go test ./... -race -count=1
go vet ./...
python -m pytest client/python/tests -q
```

## 限制

当前生产数据面仍为单节点；可恢复分片会话和回收站支持服务重启后继续，但尚未替代副本、自动故障转移或跨区域容灾。必须通过主机级备份保护 `/var/lib/astrastore-xion`。历史博客媒体本轮不迁移。

## License

MIT
