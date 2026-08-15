# Xion 可恢复分片上传设计

## 目标

将 Xion 当前的一次性 multipart 上传扩展为可持久化、可查询、可恢复的分片上传，优先解决 1 GiB 文件在网络中断后必须重新上传的问题，并为后续复制任务复用稳定的上传会话状态。

本阶段不改变现有 `POST /api/v1/files` 契约；旧客户端继续使用单次上传。新增接口只使用服务令牌，博客和 SDK 可以逐步切换。

## 范围

- 创建上传会话时声明文件名、内容类型、元数据、总大小和可选 SHA-256。
- 分片按字节偏移顺序写入磁盘，服务重启后保留已接收字节数。
- 查询会话状态，客户端根据 `received_bytes` 继续上传。
- 完成时校验总大小和可选 SHA-256，再原子生成标准 Xion 文件对象。
- 中止会话并删除临时数据。
- 启动就绪检查识别未知上传会话、临时清单和不完整分片文件；提供显式清理接口，避免自动删除用户仍可能恢复的会话。
- 返回明确的冲突、校验失败、容量暂停和上传未完成错误。

不在本阶段实现并行乱序分片、对象版本/回收站、跨节点复制、S3 全量兼容或浏览器直传；这些功能会建立在本阶段的会话模型之上。

## 存储布局

```text
<data>/uploads/<upload-id>/data.part
<data>/uploads/<upload-id>/manifest.json
<data>/uploads/<upload-id>/manifest.json.tmp
```

上传会话使用 UUID。`manifest.json` 包含会话 ID、文件属性、期望大小、已接收字节数、期望校验和、状态和更新时间。每次追加先写入并同步数据，再原子更新清单；清单更新失败时不会增加已接收偏移。

## API

```text
POST /api/v1/uploads
Content-Type: application/json
{
  "filename": "large.zip",
  "content_type": "application/zip",
  "size": 104857600,
  "checksum": "optional sha256",
  "metadata": {"owner": "blog"}
}

GET /api/v1/uploads/{id}

PUT /api/v1/uploads/{id}
Content-Range: bytes <received>-<end>/<total>
X-Chunk-Checksum: optional sha256 of this request body
<raw chunk bytes>

POST /api/v1/uploads/{id}/complete

DELETE /api/v1/uploads/{id}
```

分片必须从服务返回的 `received_bytes` 开始；偏移不匹配返回 `409 upload_offset_conflict`。完成前总大小必须完全接收。完成后响应使用现有 `files.File`，对象可通过原有下载、列表和删除接口访问。

## 错误与安全

- 会话 ID 严格使用 UUID 校验，路径不能穿越。
- 总大小必须大于 0 且不超过服务端配置的最大上传大小。
- 单个分片限制为服务端最大上传大小，实际请求由 `Content-Range` 和读取限制共同约束。
- 每次写入检查容量暂停状态；暂停时返回现有 507 `storage_paused`，已创建会话仍可查询和删除。
- 完成时重新计算完整 SHA-256，避免仅信任客户端声明。
- 下载继续强制附件响应，分片接口不接受 multipart 嵌套请求。

## 测试与兼容性

- 存储层测试覆盖创建、追加、重启恢复、偏移冲突、大小/校验和校验、完成原子性和中止清理。
- HTTP 测试覆盖鉴权、会话状态、分片流式写入、错误状态码和旧上传接口回归。
- `go test ./...`、`go vet ./...` 必须通过。
- 现有 SDK 和博客 API 不需要立即升级；后续通过同一会话接口增加 SDK/前端适配。
