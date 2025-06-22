#!/bin/bash

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 停止API网关
if [ -f logs/apigateway/apigateway.pid ]; then
    PID=$(cat logs/apigateway/apigateway.pid)
    echo -e "${GREEN}停止API网关 (PID: $PID)...${NC}"
    kill -15 $PID 2>/dev/null || echo -e "${RED}无法停止API网关${NC}"
else
    echo -e "${RED}API网关PID文件不存在${NC}"
fi

# 停止元数据服务
if [ -f logs/metaservice/metaservice.pid ]; then
    PID=$(cat logs/metaservice/metaservice.pid)
    echo -e "${GREEN}停止元数据服务 (PID: $PID)...${NC}"
    kill -15 $PID 2>/dev/null || echo -e "${RED}无法停止元数据服务${NC}"
else
    echo -e "${RED}元数据服务PID文件不存在${NC}"
fi

# 停止存储节点1
if [ -f logs/storagenode1/storagenode.pid ]; then
    PID=$(cat logs/storagenode1/storagenode.pid)
    echo -e "${GREEN}停止存储节点1 (PID: $PID)...${NC}"
    kill -15 $PID 2>/dev/null || echo -e "${RED}无法停止存储节点1${NC}"
else
    echo -e "${RED}存储节点1 PID文件不存在${NC}"
fi

# 停止存储节点2
if [ -f logs/storagenode2/storagenode.pid ]; then
    PID=$(cat logs/storagenode2/storagenode.pid)
    echo -e "${GREEN}停止存储节点2 (PID: $PID)...${NC}"
    kill -15 $PID 2>/dev/null || echo -e "${RED}无法停止存储节点2${NC}"
else
    echo -e "${RED}存储节点2 PID文件不存在${NC}"
fi

# 停止存储节点3
if [ -f logs/storagenode3/storagenode.pid ]; then
    PID=$(cat logs/storagenode3/storagenode.pid)
    echo -e "${GREEN}停止存储节点3 (PID: $PID)...${NC}"
    kill -15 $PID 2>/dev/null || echo -e "${RED}无法停止存储节点3${NC}"
else
    echo -e "${RED}存储节点3 PID文件不存在${NC}"
fi

# 停止Docker服务
echo -e "${GREEN}停止Docker服务...${NC}"
docker-compose -f deploy/docker-compose.yaml down

echo -e "${GREEN}所有服务已停止${NC}" 