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
	port       = flag.String("port", "9000", "元数据服务监听端口")
	configFile = flag.String("config", "configs/metaservice.yaml", "配置文件路径")
)

func main() {
	flag.Parse()

	// 加载配置文件
	log.Printf("加载配置文件: %s", *configFile)
	// TODO: 加载配置文件

	// 创建gRPC服务器
	grpcServer := grpc.NewServer()

	// 注册服务
	// TODO: 注册元数据服务

	// 启动服务器
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("无法监听端口 %s: %v", *port, err)
	}

	// 启动gRPC服务器
	go func() {
		log.Printf("元数据服务启动在端口: %s", *port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("无法启动服务器: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("关闭服务器...")

	// 停止gRPC服务器
	grpcServer.GracefulStop()
	log.Println("服务器已关闭")
}
