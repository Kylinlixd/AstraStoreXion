# Xion 异步复制队列设计

## 目标

为 Xion 增加一个默认关闭、可选配置的主节点到副节点异步复制队列。上传、软删除和恢复只在本节点成功后返回，复制在后台执行并持久化任务状态；副节点不可用时主节点仍可读写，任务进入重试状态。

## 配置

```text
XION_REPLICA_URL=http://127.0.0.1:18081
XION_REPLICA_TOKEN=<副节点服务令牌>
XION_REPLICATION_DIR=<数据目录>/replication
XION_REPLICATION_MAX_ATTEMPTS=10
```

未设置 `XION_REPLICA_URL` 时不启动复制器。复制请求使用服务令牌，并添加内部 `X-Xion-Replication: true` 标记；副节点不会再次把该请求加入自己的复制队列，避免循环复制。

## 任务模型

```text
<data>/replication/<operation>-<file-id>.json
```

任务操作为 `upload`、`delete` 或 `restore`，状态为 `pending`、`running`、`failed`、`completed`。任务清单原子写入，服务重启后 `running` 会恢复为 `pending`。相同操作和文件 ID 幂等，不重复创建任务。

## 处理策略

- 上传任务从本节点流式读取文件，通过现有 multipart 文件接口发送到副节点。
- 上传请求携带主节点文件 ID；副节点使用该 ID 落盘，重试前先按 ID 和校验和检查，避免网络超时后的重复对象。
- 删除任务调用副节点删除接口；副节点同样进入回收站。
- 恢复任务调用副节点恢复接口。
- 网络错误和 5xx 按递增等待重试，达到最大次数后保留 `failed` 状态，支持重新触发。
- 复制失败不回滚主节点写入；状态接口显示待同步数量和最后错误。

## API

```text
GET  /api/v1/replication
POST /api/v1/replication/{file-id}/retry
```

接口需要服务令牌。状态响应不包含服务令牌和内部地址以外的敏感配置。

## 范围边界

本阶段不实现自动主节点选举、双写强一致、纠删码和跨地域拓扑；复制队列先为后续故障切换提供可观测的 RPO 基础。
