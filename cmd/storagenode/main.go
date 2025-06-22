package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

var (
	port       = flag.String("port", "9100", "存储节点监听端口")
	configFile = flag.String("config", "configs/storagenode.yaml", "配置文件路径")
	dataDir    = flag.String("data", "data", "数据存储目录")
	nodeID     = flag.String("id", "", "节点ID")
)

func main() {
	flag.Parse()

	// 检查节点ID
	if *nodeID == "" {
		log.Fatal("必须指定节点ID")
	}

	// 加载配置文件
	log.Printf("加载配置文件: %s", *configFile)
	// TODO: 加载配置文件

	// 确保数据目录存在
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("无法创建数据目录: %v", err)
	}

	// 创建gRPC服务器
	grpcServer := grpc.NewServer()

	// 注册服务
	// TODO: 注册存储节点服务

	// 启动服务器
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("无法监听端口 %s: %v", *port, err)
	}

	// 启动gRPC服务器
	go func() {
		log.Printf("存储节点(ID:%s)启动在端口: %s", *nodeID, *port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("无法启动服务器: %v", err)
		}
	}()

	// 启动心跳
	// TODO: 启动心跳

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("关闭存储节点...")

	// 停止心跳
	// TODO: 停止心跳

	// 停止gRPC服务器
	grpcServer.GracefulStop()
	log.Println("存储节点已关闭")
}
