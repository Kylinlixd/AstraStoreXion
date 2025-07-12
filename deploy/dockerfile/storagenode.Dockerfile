FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o storagenode ./core/storagenode

# 使用scratch作为基础镜像
FROM alpine:latest

WORKDIR /app

# 从builder阶段复制二进制文件
COPY --from=builder /app/storagenode /app/

# 创建配置文件、日志和数据目录
RUN mkdir -p /app/configs /app/logs /app/data /app/raft_data

# 设置环境变量
ENV PORT=9100 \
    CONFIG_FILE=/app/configs/storagenode.yaml \
    NODE_ID=node1 \
    DATA_DIR=/app/data

# 暴露端口
EXPOSE 9100 9200

# 运行应用
CMD ["/app/storagenode", "-port", "${PORT}", "-config", "${CONFIG_FILE}", "-id", "${NODE_ID}", "-data", "${DATA_DIR}"] 