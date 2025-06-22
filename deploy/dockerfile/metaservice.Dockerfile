FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o metaservice ./cmd/metaservice

# 使用scratch作为基础镜像
FROM alpine:latest

WORKDIR /app

# 从builder阶段复制二进制文件
COPY --from=builder /app/metaservice /app/

# 创建配置文件和日志目录
RUN mkdir -p /app/configs /app/logs

# 设置环境变量
ENV PORT=9000 \
    CONFIG_FILE=/app/configs/metaservice.yaml

# 暴露端口
EXPOSE 9000

# 运行应用
CMD ["/app/metaservice", "-port", "${PORT}", "-config", "${CONFIG_FILE}"] 