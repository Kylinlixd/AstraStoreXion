#!/bin/bash

# 创建需要的目录
mkdir -p data logs

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# 检查二进制文件是否存在
if [ ! -f bin/apigateway ] || [ ! -f bin/metaservice ] || [ ! -f bin/storagenode ]; then
    echo -e "${YELLOW}二进制文件不存在，正在构建...${NC}"
    ./scripts/build.sh
fi

# 检查Docker Compose是否已安装
if ! command -v docker-compose &> /dev/null
then
    echo -e "${RED}Docker Compose未安装，请先安装${NC}"
    exit 1
fi

# 启动依赖服务（Consul、Etcd、MySQL）
echo -e "${GREEN}启动依赖服务...${NC}"
docker-compose -f deploy/docker-compose.yaml up -d consul etcd mysql
sleep 5 # 等待服务启动

# 启动元数据服务
echo -e "${GREEN}启动元数据服务...${NC}"
mkdir -p logs/metaservice
bin/metaservice -config configs/metaservice.yaml > logs/metaservice/metaservice.log 2>&1 &
META_PID=$!
echo $META_PID > logs/metaservice/metaservice.pid
echo -e "${GREEN}元数据服务启动成功，PID: $META_PID${NC}"

# 启动存储节点
echo -e "${GREEN}启动存储节点1...${NC}"
mkdir -p logs/storagenode1 data/node1
bin/storagenode -id node1 -port 9101 -config configs/storagenode.yaml -data data/node1 > logs/storagenode1/storagenode.log 2>&1 &
NODE1_PID=$!
echo $NODE1_PID > logs/storagenode1/storagenode.pid
echo -e "${GREEN}存储节点1启动成功，PID: $NODE1_PID${NC}"

echo -e "${GREEN}启动存储节点2...${NC}"
mkdir -p logs/storagenode2 data/node2
bin/storagenode -id node2 -port 9102 -config configs/storagenode.yaml -data data/node2 > logs/storagenode2/storagenode.log 2>&1 &
NODE2_PID=$!
echo $NODE2_PID > logs/storagenode2/storagenode.pid
echo -e "${GREEN}存储节点2启动成功，PID: $NODE2_PID${NC}"

echo -e "${GREEN}启动存储节点3...${NC}"
mkdir -p logs/storagenode3 data/node3
bin/storagenode -id node3 -port 9103 -config configs/storagenode.yaml -data data/node3 > logs/storagenode3/storagenode.log 2>&1 &
NODE3_PID=$!
echo $NODE3_PID > logs/storagenode3/storagenode.pid
echo -e "${GREEN}存储节点3启动成功，PID: $NODE3_PID${NC}"

# 启动API网关
echo -e "${GREEN}启动API网关...${NC}"
mkdir -p logs/apigateway
bin/apigateway -config configs/apigateway.yaml > logs/apigateway/apigateway.log 2>&1 &
API_PID=$!
echo $API_PID > logs/apigateway/apigateway.pid
echo -e "${GREEN}API网关启动成功，PID: $API_PID${NC}"

echo -e "${GREEN}所有服务已启动${NC}"
echo "查看日志:"
echo "  API网关: tail -f logs/apigateway/apigateway.log"
echo "  元数据服务: tail -f logs/metaservice/metaservice.log"
echo "  存储节点1: tail -f logs/storagenode1/storagenode.log"
echo "  存储节点2: tail -f logs/storagenode2/storagenode.log"
echo "  存储节点3: tail -f logs/storagenode3/storagenode.log" 