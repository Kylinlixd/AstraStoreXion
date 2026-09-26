# 生产形态：单节点 astrastore-xion（静态二进制 + 数据卷）。
#
# 之前的三个 Dockerfile 只构建分布式原型里的 apigateway/metaservice/storagenode，
# 没有一个能产出生产实际运行的服务。这个文件补齐部署形态与代码形态的一致。
FROM golang:1.23-alpine AS builder

ARG VERSION=dev
ARG COMMIT=none

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags="-X main.version=${VERSION} -X main.commit=${COMMIT}" \
        -o /out/astrastore-xion ./core/apigateway \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags="-X main.version=${VERSION} -X main.commit=${COMMIT} -s -w" \
        -o /out/xionctl ./cmd/xionctl

FROM alpine:3.20
RUN adduser -D -H -u 10001 astrastore-xion \
    && mkdir -p /var/lib/astrastore-xion \
    && chown astrastore-xion:astrastore-xion /var/lib/astrastore-xion

COPY --from=builder /out/astrastore-xion /usr/local/bin/astrastore-xion
COPY --from=builder /out/xionctl /usr/local/bin/xionctl

USER astrastore-xion
ENV XION_LISTEN_ADDR=0.0.0.0:8081 \
    XION_DATA_DIR=/var/lib/astrastore-xion \
    XION_STORAGE_PAUSE_AT_PERCENT=90 \
    XION_UPLOAD_SESSION_TTL=24h
VOLUME ["/var/lib/astrastore-xion"]
EXPOSE 8081

# XION_SERVICE_TOKEN 必须由运行时提供，不能打进镜像。
ENTRYPOINT ["/usr/local/bin/astrastore-xion"]
