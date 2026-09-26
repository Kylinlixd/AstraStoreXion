VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)
XIONCTL_LDFLAGS := $(LDFLAGS) -s -w

.PHONY: all build clean test test-unit test-integration bench cover gen-proto apigateway xion-service xionctl metaservice storagenode client python-test fixtures smoke docker-build docker-build-experimental release-linux dev-start dev-stop help

# 默认目标
all: build

# 构建所有组件
build:
	@echo "构建所有组件..."
	@mkdir -p bin
	@go build -o bin/apigateway ./core/apigateway
	@go build -o bin/metaservice ./core/metaservice
	@go build -o bin/storagenode ./core/storagenode
	@go build -o bin/xion-client ./client/go/cmd
	@echo "构建完成！"

# 构建API网关
apigateway:
	@echo "构建API网关..."
	@mkdir -p bin
	@go build -ldflags='$(LDFLAGS)' -o bin/apigateway ./core/apigateway

# 构建博客融合使用的单节点服务
xion-service:
	@mkdir -p bin
	@go build -trimpath -ldflags='$(LDFLAGS)' -o bin/astrastore-xion ./core/apigateway

# 构建服务器命令行工具
xionctl:
	@mkdir -p bin
	@go build -trimpath -ldflags='$(XIONCTL_LDFLAGS)' -o bin/xionctl ./cmd/xionctl

# 交叉编译 Linux amd64 发布产物
release-linux:
	@mkdir -p bin/linux-amd64
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='$(LDFLAGS)' -o bin/linux-amd64/astrastore-xion ./core/apigateway
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='$(XIONCTL_LDFLAGS)' -o bin/linux-amd64/xionctl ./cmd/xionctl
	@echo "发布产物: bin/linux-amd64 (版本 $(VERSION))"

# 运行生产 Python SDK 测试
python-test:
	@python3 -m pytest client/python/tests -q

# 生成 PNG、PDF、DOCX、TXT 与 checksum manifest
fixtures:
	@python3 scripts/generate_test_artifacts.py

# 对指定测试文件运行完整生命周期；需传 FIXTURE 和环境变量
smoke:
	@test -n "$(FIXTURE)" || (echo "FIXTURE is required" >&2; exit 2)
	@./scripts/smoke-test.sh "$(FIXTURE)"

# 构建元数据服务
metaservice:
	@echo "构建元数据服务..."
	@mkdir -p bin
	@go build -o bin/metaservice ./core/metaservice

# 构建存储节点
storagenode:
	@echo "构建存储节点..."
	@mkdir -p bin
	@go build -o bin/storagenode ./core/storagenode

# 构建客户端工具
client:
	@echo "构建客户端工具..."
	@mkdir -p bin
	@go build -o bin/xion-client ./client/go/cmd

# 运行测试
test:
	@echo "运行所有测试..."
	@go test -v ./...

# 运行单元测试
test-unit:
	@echo "运行单元测试..."
	@go test -v ./test/unit/...

# 运行集成测试
test-integration:
	@echo "运行集成测试..."
	@go test -v ./test/integration/...

# 运行基准测试
bench:
	@echo "运行基准测试..."
	@go test -bench=. -benchmem ./...

# 运行测试覆盖率
cover:
	@echo "运行测试覆盖率分析..."
	@go test -cover -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "测试覆盖率报告已生成: coverage.html"

# 生成Protocol Buffers代码
gen-proto:
	@echo "生成Protocol Buffers代码..."
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		pkg/api/*.proto
	@echo "Protocol Buffers代码生成完成！"

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf bin

# 构建生产镜像（单节点文件服务）
docker-build:
	@echo "构建生产镜像..."
	@docker build -t astrastore/xion:$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		-f deploy/dockerfile/astrastore-xion.Dockerfile .
	@echo "镜像构建完成: astrastore/xion:$(VERSION)"

# 构建实验性分布式原型镜像（不是生产依赖）
docker-build-experimental:
	@echo "构建实验性镜像..."
	@docker build -t astrastore/apigateway:latest -f deploy/dockerfile/apigateway.Dockerfile .
	@docker build -t astrastore/metaservice:latest -f deploy/dockerfile/metaservice.Dockerfile .
	@docker build -t astrastore/storagenode:latest -f deploy/dockerfile/storagenode.Dockerfile .
	@echo "实验性镜像构建完成！"

# 启动开发环境
dev-start:
	@echo "启动开发环境..."
	@./scripts/dev-start.sh

# 停止开发环境
dev-stop:
	@echo "停止开发环境..."
	@./scripts/dev-stop.sh

# 显示帮助信息
help:
	@echo "星辰离子X (AstraStoreXion) 项目构建工具"
	@echo ""
	@echo "可用命令:"
	@echo "  make                 - 构建所有组件"
	@echo "  make apigateway      - 只构建API网关"
	@echo "  make xion-service    - 构建博客融合单节点服务"
	@echo "  make xionctl         - 构建服务器文件操作命令"
	@echo "  make metaservice     - 只构建元数据服务"
	@echo "  make storagenode     - 只构建存储节点"
	@echo "  make client          - 只构建客户端工具"
	@echo "  make python-test     - 运行 Python SDK 测试"
	@echo "  make fixtures        - 生成上传验收资料"
	@echo "  make smoke FIXTURE=... - 运行字节级生命周期测试"
	@echo "  make test            - 运行所有测试"
	@echo "  make test-unit       - 只运行单元测试"
	@echo "  make test-integration - 只运行集成测试"
	@echo "  make bench           - 运行基准测试"
	@echo "  make cover           - 生成测试覆盖率报告"
	@echo "  make gen-proto       - 生成Protocol Buffers代码"
	@echo "  make clean           - 清理构建文件"
	@echo "  make docker-build    - 构建生产 Docker 镜像"
	@echo "  make docker-build-experimental - 构建实验性原型镜像"
	@echo "  make dev-start       - 启动开发环境"
	@echo "  make dev-stop        - 停止开发环境"
	@echo "  make help            - 显示帮助信息"
