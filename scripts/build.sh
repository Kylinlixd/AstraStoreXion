#!/bin/bash

# 检查Go环境
if ! command -v go &> /dev/null
then
    echo "Go未安装，请先安装Go"
    exit 1
fi

# 清理旧的构建
rm -rf bin
mkdir -p bin

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 构建API网关
echo "构建API网关..."
go build -o bin/apigateway ./core/apigateway
if [ $? -eq 0 ]; then
    echo -e "${GREEN}API网关构建成功${NC}"
else
    echo -e "${RED}API网关构建失败${NC}"
    exit 1
fi

# 构建元数据服务
echo "构建元数据服务..."
go build -o bin/metaservice ./core/metaservice
if [ $? -eq 0 ]; then
    echo -e "${GREEN}元数据服务构建成功${NC}"
else
    echo -e "${RED}元数据服务构建失败${NC}"
    exit 1
fi

# 构建存储节点
echo "构建存储节点..."
go build -o bin/storagenode ./core/storagenode
if [ $? -eq 0 ]; then
    echo -e "${GREEN}存储节点构建成功${NC}"
else
    echo -e "${RED}存储节点构建失败${NC}"
    exit 1
fi

echo -e "${GREEN}所有组件构建成功！${NC}"
echo "二进制文件位于 bin/ 目录" 