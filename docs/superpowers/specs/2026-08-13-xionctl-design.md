# xionctl 服务器命令行设计

## 目标

为 AstraStoreXion 增加一个可安装到服务器 `/usr/local/bin/xionctl` 的命令行工具，让运维人员无需手写 `curl` 就能查看和操作 Xion 文件。

## 用户体验

默认从 `/etc/astrastore-xion.env` 读取 `XION_LISTEN_ADDR` 与 `XION_SERVICE_TOKEN`，也支持环境变量和命令行参数覆盖：

```bash
xionctl health
xionctl list
xionctl info <file-id>
xionctl upload ./example.png
xionctl download <file-id> ./example.png
xionctl delete <file-id> --yes
```

删除必须显式提供 `--yes`；下载先写入同目录临时文件，成功后再原子替换目标文件，避免网络中断留下半个文件。命令输出面向 shell 使用，列表和详情使用 JSON，健康检查使用简短文本。

## 架构

- `cmd/xionctl`：独立 Go CLI，使用标准库 `flag`、`net/http`、`mime/multipart`，不依赖现有实验性 Go client stub。
- `config`：按优先级读取命令行参数、进程环境、env 文件；默认 API 地址由 `XION_LISTEN_ADDR` 组成，默认 env 文件为 `/etc/astrastore-xion.env`。
- `client`：封装 `/healthz`、`/readyz`、`/api/v1/files` 的认证 HTTP 请求，统一解析 Xion 结构化错误。
- `commands`：实现 health、list、info、upload、download、delete，并将错误写到 stderr、以非零状态退出。

## 安全与边界

- 服务 token 只放在请求头，不写入输出；读取 env 文件时不打印内容。
- API 默认只连接 loopback；显式传入远程 URL 是操作者责任。
- 文件 ID 直接作为 URL path segment 前先做非空校验；下载目标路径拒绝目录并使用临时文件。
- CLI 只操作 Xion API，不直接读取或删除 `/var/lib/astrastore-xion/objects`，避免绕过 manifest 与完整性校验。

## 验证

- httptest 覆盖配置解析、认证请求、列表/详情、上传 multipart、下载原子写入、删除确认和错误退出路径。
- `go test ./cmd/xionctl ./...`、`go vet ./...`、`make xionctl` 必须通过。
- 文档给出服务器安装、env 文件权限和常用操作示例。
