# 星辰离子X (AstraStoreXion)

## 项目概述

星辰离子X是一个高性能分布式文件存储系统，旨在提高网站资源加载速度，支持水平扩展，保证高可用性和数据一致性，并实现低延迟读写。

### 主要特点

- **高性能**: 优化存储架构和读写策略，减少文件访问延迟
- **水平扩展**: 支持通过添加存储节点来增加存储容量和处理能力
- **高可用性**: 采用冗余存储和分布式协议，确保节点故障时数据不丢失
- **数据一致性**: 使用Raft协议实现副本间的数据一致性
- **低延迟读写**: 通过优化存储引擎和网络通信，减少读写操作响应时间

## 技术栈

- **通信框架**: gRPC + Protocol Buffers
- **服务发现**: Consul
- **分布式协调**: Etcd
- **监控**: Prometheus + Grafana
- **容器化部署**: Docker, Kubernetes

## 项目结构

```
AstraStoreXion/
├── core/                # 应用程序入口点
│   ├── apigateway/      # API网关程序入口
│   ├── metaservice/     # 元数据服务程序入口
│   └── storagenode/     # 存储节点程序入口
├── pkg/                 # 共享库
│   ├── api/             # API定义（包含Protocol Buffers）
│   ├── metadata/        # 元数据模块
│   ├── storage/         # 存储引擎模块
│   ├── balance/         # 数据平衡模块
│   └── raft/            # Raft一致性模块
├── client/              # 客户端SDK
│   ├── go/              # Go客户端
│   ├── java/            # Java客户端
│   ├── nodejs/          # NodeJS客户端
│   └── python/          # Python客户端
├── configs/             # 配置文件
├── scripts/             # 部署、测试脚本
├── test/                # 测试代码
│   ├── unit/            # 单元测试
│   └── integration/     # 集成测试
├── deploy/              # 部署配置
│   ├── dockerfile/      # Docker镜像构建文件
│   ├── mysql/           # MySQL数据库初始化脚本
│   └── docker-compose.yaml  # Docker Compose配置
└── docs/                # 文档
```

## 项目进展

### 已完成功能

- ✅ **核心架构设计**：三层架构（接入层、元数据层、存储层）
- ✅ **API设计**：基于gRPC的文件服务API
- ✅ **客户端SDK**：
  - ✅ Go客户端：基本API实现
  - ✅ Java客户端：基本API实现
  - ✅ NodeJS客户端：基本API实现
  - ✅ Python客户端：基本API实现
- ✅ **存储引擎**：
  - ✅ 本地存储引擎：支持块级存储和磁盘使用率监控
  - ✅ 心跳监测：节点状态监控
- ✅ **元数据服务**：
  - ✅ 内存实现：用于开发和测试
- ✅ **数据平衡**：
  - ✅ 一致性哈希负载均衡器：节点添加/移除，数据分片
  - ✅ 热点文件检测：文件访问统计与热点识别
- ✅ **共识模块**：
  - ✅ Raft协议实现：基于Hashicorp Raft
  - ✅ 节点状态管理：Leader选举、日志复制
- ✅ **测试框架**：
  - ✅ 单元测试：核心模块测试
  - ✅ 集成测试：存储节点测试
- ✅ **文档**：
  - ✅ 客户端使用文档
  - ✅ 项目结构文档

### 待完成功能

- 🔄 **API网关改进**：
  - 完善请求路由和负载均衡
  - 添加认证与授权机制
  - 实现请求限流与熔断
- 🔄 **元数据服务持久化**：
  - MySQL存储实现
  - 元数据缓存层
- 🔄 **存储引擎优化**：
  - 数据压缩
  - 垃圾回收机制
  - 存储分层策略
- 🔄 **数据平衡优化**：
  - 自动化数据迁移实现
  - 负载均衡策略调优
- 🔄 **监控系统**：
  - Prometheus + Grafana监控集成
  - 系统性能指标收集
  - 告警机制
- 🔄 **安全机制**：
  - 数据加密
  - 访问控制
  - 审计日志
- 🔄 **容灾备份**：
  - 定期快照
  - 跨区域备份
- 🔄 **CI/CD流程**：
  - 自动化测试流程
  - 自动化部署流程

## 使用说明

### 安装

1. 克隆仓库
```bash
git clone https://github.com/astrastore/astrastore-xion.git
cd astrastore-xion
```

2. 使用Docker Compose启动服务
```bash
docker-compose -f deploy/docker-compose.yaml up -d
```

### 开发环境搭建

1. 安装依赖
```bash
# 安装Go依赖
go mod tidy
```

2. 启动开发环境
```bash
# 启动开发环境
./scripts/dev-start.sh
```

3. 运行测试
```bash
# 运行单元测试
go test ./test/unit/...

# 运行集成测试
go test ./test/integration/...
```

## 客户端使用

请参阅 [客户端使用文档](./docs/客户端使用文档.md)。

## 贡献指南

1. Fork项目仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建Pull Request

## 许可证

MIT 