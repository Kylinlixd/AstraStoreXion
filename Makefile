.PHONY: all build clean test test-unit test-integration bench cover gen-proto apigateway metaservice storagenode client docker-build dev-start dev-stop help

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
	@go build -o bin/apigateway ./core/apigateway

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

# 构建Docker镜像
docker-build:
	@echo "构建Docker镜像..."
	@docker build -t astrastore/apigateway:latest -f deploy/dockerfile/apigateway.Dockerfile .
	@docker build -t astrastore/metaservice:latest -f deploy/dockerfile/metaservice.Dockerfile .
	@docker build -t astrastore/storagenode:latest -f deploy/dockerfile/storagenode.Dockerfile .
	@echo "Docker镜像构建完成！"

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
	@echo "  make metaservice     - 只构建元数据服务"
	@echo "  make storagenode     - 只构建存储节点"
	@echo "  make client          - 只构建客户端工具"
	@echo "  make test            - 运行所有测试"
	@echo "  make test-unit       - 只运行单元测试"
	@echo "  make test-integration - 只运行集成测试"
	@echo "  make bench           - 运行基准测试"
	@echo "  make cover           - 生成测试覆盖率报告"
	@echo "  make gen-proto       - 生成Protocol Buffers代码"
	@echo "  make clean           - 清理构建文件"
	@echo "  make docker-build    - 构建Docker镜像"
	@echo "  make dev-start       - 启动开发环境"
	@echo "  make dev-stop        - 停止开发环境"
	@echo "  make help            - 显示帮助信息" 