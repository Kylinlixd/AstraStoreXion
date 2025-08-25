# 星辰离子X 运维手册

本文档为星辰离子X分布式文件存储系统的运维手册，包括系统部署、配置、监控、故障排除和维护等内容。

## 目录

- [系统架构](#系统架构)
- [部署指南](#部署指南)
- [配置管理](#配置管理)
- [监控与告警](#监控与告警)
- [日志管理](#日志管理)
- [故障排除](#故障排除)
- [性能调优](#性能调优)
- [备份与恢复](#备份与恢复)
- [安全管理](#安全管理)
- [升级指南](#升级指南)

## 系统架构

星辰离子X采用三层架构设计：

1. **接入层（API网关）**：处理客户端请求，提供统一的API接口，负责认证授权、限流熔断等。
2. **元数据层（元数据服务）**：管理文件元数据，包括文件位置、大小、权限等信息。
3. **存储层（存储节点）**：负责实际的文件块存储，支持水平扩展。

系统还包括以下关键组件：

- **负载均衡器**：基于一致性哈希算法的负载均衡，确保数据分布均匀。
- **共识模块**：基于Raft协议的共识机制，确保元数据的一致性。
- **监控系统**：基于Prometheus和Grafana的监控告警系统。

## 部署指南

### 系统要求

- **操作系统**：Linux（推荐Ubuntu 20.04+或CentOS 8+）
- **CPU**：每个节点至少4核
- **内存**：每个节点至少8GB
- **存储**：根据需求配置，建议SSD
- **网络**：千兆网络或更高

### Docker部署

1. 安装Docker和Docker Compose：

```bash
# 安装Docker
curl -fsSL https://get.docker.com | sh

# 安装Docker Compose
curl -L "https://github.com/docker/compose/releases/download/v2.10.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose
```

2. 克隆代码仓库：

```bash
git clone https://github.com/astrastore/astrastore-xion.git
cd astrastore-xion
```

3. 修改配置文件：

```bash
# 根据环境修改配置文件
vim deploy/docker-compose.yaml
vim configs/apigateway.yaml
vim configs/metaservice.yaml
vim configs/storagenode.yaml
```

4. 启动服务：

```bash
docker-compose -f deploy/docker-compose.yaml up -d
```

### Kubernetes部署

1. 准备Kubernetes集群（1.20+）。
2. 安装Helm（3.0+）。
3. 添加Helm仓库：

```bash
helm repo add astrastore https://helm.astrastore.io
helm repo update
```

4. 安装星辰离子X：

```bash
helm install xion astrastore/astrastore-xion \
  --namespace xion \
  --create-namespace \
  --set apiGateway.replicas=3 \
  --set metaService.replicas=3 \
  --set storageNode.replicas=5
```

## 配置管理

### 配置文件

系统使用YAML格式的配置文件，主要包括：

- `configs/apigateway.yaml`：API网关配置
- `configs/metaservice.yaml`：元数据服务配置
- `configs/storagenode.yaml`：存储节点配置

### API网关配置示例

```yaml
server:
  port: 8080
  read_timeout: 60s
  write_timeout: 60s
  idle_timeout: 120s

auth:
  jwt_secret: "your-secret-key"
  token_expiry: 24h
  refresh_expiry: 168h

rate_limit:
  ip_limit: 100
  user_limit: 300
  upload_limit: 10

monitor:
  port: 9090
  enable_metrics: true
  enable_tracing: true
  jaeger_endpoint: "http://jaeger:14268/api/traces"
```

### 元数据服务配置示例

```yaml
server:
  port: 9000

database:
  type: "mysql"
  host: "mysql"
  port: 3306
  user: "xion"
  password: "password"
  database: "xion_metadata"
  max_open_conns: 100
  max_idle_conns: 10
  conn_max_lifetime: 1h

raft:
  node_id: "meta1"
  data_dir: "/data/raft"
  peers: ["meta1", "meta2", "meta3"]
  heartbeat_timeout: 1000
  election_timeout: 1000
```

### 存储节点配置示例

```yaml
server:
  port: 9100

storage:
  node_id: "node1"
  data_path: "/data/storage"
  disk_quota: 1073741824000  # 1TB
  chunk_size: 4194304  # 4MB
  heartbeat_ttl: 3

compression:
  enable: true
  algorithm: "zstd"
  level: 3

encryption:
  enable: true
  algorithm: "aes"
  key_size: 32
```

## 监控与告警

### Prometheus指标

系统通过Prometheus导出以下关键指标：

- **API网关指标**：
  - `apigateway_requests_total`：请求总数
  - `apigateway_request_duration_seconds`：请求处理时间
  - `apigateway_active_requests`：活跃请求数

- **元数据服务指标**：
  - `metaservice_operations_total`：操作总数
  - `metaservice_operation_duration_seconds`：操作处理时间
  - `metaservice_db_connections`：数据库连接数

- **存储节点指标**：
  - `storagenode_disk_usage_percent`：磁盘使用率
  - `storagenode_chunks_total`：块总数
  - `storagenode_read_write_ops`：读写操作数

### Grafana仪表盘

系统提供以下Grafana仪表盘：

- **系统概览**：显示整体系统状态、请求量、错误率等。
- **API网关监控**：详细的API请求指标和性能数据。
- **元数据服务监控**：元数据操作和数据库性能指标。
- **存储节点监控**：存储节点的磁盘使用率、IO性能等。

### 告警规则

系统配置了以下关键告警规则：

- **高错误率**：5分钟内错误率超过5%。
- **高延迟**：请求处理时间超过1秒。
- **磁盘使用率**：存储节点磁盘使用率超过80%。
- **节点离线**：节点超过5分钟未发送心跳。

## 日志管理

### 日志位置

- Docker环境：日志存储在容器内的`/var/log/xion`目录。
- Kubernetes环境：日志通过标准输出收集，可通过`kubectl logs`查看。

### 日志级别

系统支持以下日志级别：

- `debug`：调试信息，开发环境使用。
- `info`：普通信息，默认级别。
- `warn`：警告信息，可能的问题。
- `error`：错误信息，需要关注。
- `fatal`：致命错误，系统无法继续运行。

### 日志格式

日志采用JSON格式，包含以下字段：

```json
{
  "timestamp": "2023-05-18T10:30:45Z",
  "level": "info",
  "message": "请求处理成功",
  "service": "apigateway",
  "trace_id": "1234567890abcdef",
  "request_id": "req-1234567890",
  "user_id": "user-001",
  "path": "/api/v1/files/123",
  "method": "GET",
  "status": 200,
  "duration_ms": 45
}
```

## 故障排除

### 常见问题

#### API网关无法访问

1. 检查API网关服务是否运行：`docker ps | grep apigateway`
2. 检查网络连接：`curl -v http://localhost:8080/health`
3. 检查日志：`docker logs xion-apigateway`

#### 文件上传失败

1. 检查存储节点状态：`docker ps | grep storagenode`
2. 检查存储节点磁盘空间：`df -h`
3. 检查API网关与存储节点的连接：`curl -v http://storagenode:9100/health`

#### 元数据服务异常

1. 检查元数据服务状态：`docker ps | grep metaservice`
2. 检查数据库连接：`mysql -h mysql -u xion -p -e "SELECT 1"`
3. 检查Raft集群状态：`curl -v http://metaservice:9000/raft/status`

### 日志分析

使用以下命令分析日志：

```bash
# 查找错误日志
grep -i error /var/log/xion/apigateway.log

# 查找特定请求的日志
grep "req-1234567890" /var/log/xion/apigateway.log

# 分析请求延迟
awk '{print $duration_ms}' /var/log/xion/apigateway.log | sort -n | uniq -c
```

### 系统诊断

系统提供以下诊断端点：

- `/health`：健康检查
- `/metrics`：Prometheus指标
- `/debug/pprof`：Go性能分析（仅在开发环境启用）

## 性能调优

### 系统参数调优

- **API网关**：
  - 增加最大连接数：修改`server.max_connections`
  - 调整超时时间：修改`server.read_timeout`和`server.write_timeout`

- **元数据服务**：
  - 优化数据库连接池：修改`database.max_open_conns`和`database.max_idle_conns`
  - 调整缓存大小：修改`cache.max_size`

- **存储节点**：
  - 调整块大小：修改`storage.chunk_size`
  - 优化压缩级别：修改`compression.level`

### 硬件推荐

- **API网关**：CPU密集型，推荐高频CPU，中等内存。
- **元数据服务**：内存密集型，推荐大内存，快速存储。
- **存储节点**：IO密集型，推荐大容量SSD/NVMe，足够内存。

## 备份与恢复

### 元数据备份

1. 配置定时备份：

```yaml
backup:
  schedule: "0 0 * * *"  # 每天0点
  retention: 7           # 保留7天
  storage: "s3://backup/metadata"
```

2. 手动触发备份：

```bash
curl -X POST http://metaservice:9000/admin/backup
```

### 数据恢复

1. 从备份恢复元数据：

```bash
curl -X POST http://metaservice:9000/admin/restore \
  -H "Content-Type: application/json" \
  -d '{"backup_id": "backup-20230518"}'
```

2. 验证恢复结果：

```bash
curl http://metaservice:9000/admin/status
```

## 安全管理

### 密钥管理

系统使用以下密钥：

- **JWT密钥**：用于认证授权，定期轮换。
- **加密密钥**：用于数据加密，高度保密。

密钥轮换流程：

1. 生成新密钥。
2. 更新配置文件。
3. 重启相关服务。

### 安全加固

1. 启用TLS：

```yaml
server:
  tls:
    enable: true
    cert_file: "/certs/server.crt"
    key_file: "/certs/server.key"
```

2. 配置防火墙：

```bash
# 只允许必要的端口
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT  # API网关
iptables -A INPUT -p tcp --dport 9000 -j ACCEPT  # 元数据服务
iptables -A INPUT -p tcp --dport 9100 -j ACCEPT  # 存储节点
iptables -A INPUT -j DROP
```

## 升级指南

### 升级准备

1. 备份所有数据。
2. 检查新版本的兼容性和变更日志。
3. 在测试环境验证升级流程。

### 升级步骤

1. 升级API网关：

```bash
docker-compose stop apigateway
docker-compose pull apigateway
docker-compose up -d apigateway
```

2. 升级元数据服务：

```bash
# 一次升级一个节点，确保Raft集群稳定
for node in meta1 meta2 meta3; do
  docker-compose stop $node
  docker-compose pull $node
  docker-compose up -d $node
  sleep 60  # 等待节点同步
done
```

3. 升级存储节点：

```bash
# 一次升级一部分节点，确保数据可用性
docker-compose stop storagenode1 storagenode2
docker-compose pull storagenode1 storagenode2
docker-compose up -d storagenode1 storagenode2
sleep 60
docker-compose stop storagenode3 storagenode4
docker-compose pull storagenode3 storagenode4
docker-compose up -d storagenode3 storagenode4
```

### 回滚流程

如果升级失败，按照以下步骤回滚：

1. 停止新版本服务。
2. 恢复旧版本镜像。
3. 启动旧版本服务。
4. 如果需要，从备份恢复数据。 