# AstraStoreXion

AstraStoreXion 是一个面向自托管博客的持久化文件服务。当前生产可用形态为单节点：Go HTTP 服务负责文件字节、原始名称与 SHA-256，Django 负责公网认证和业务元数据，Vue 管理端提供上传进度和内置教程。

## 当前能力

- 原子上传：对象与 JSON manifest 分离落盘，失败自动补偿。
- 重启持久化：服务重启后可继续查询和下载。
- 安全 ID：拒绝路径穿越和非法对象 ID。
- 完整生命周期：上传、列表、状态、下载、幂等删除。
- 服务端认证：常量时间比较 Bearer 服务密钥。
- 50 MB 默认限制和结构化错误响应。
- Python SDK 1.1：流式下载、安全重试和结构化异常。
- systemd 最小权限部署与字节级 smoke test。
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

当前为单节点服务，不提供副本、自动故障转移或跨区域容灾；必须通过主机级备份保护 `/var/lib/astrastore-xion`。历史博客媒体本轮不迁移。

## License

MIT
