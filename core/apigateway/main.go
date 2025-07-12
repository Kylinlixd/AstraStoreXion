package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

var (
	port       = flag.String("port", "8080", "API网关监听端口")
	configFile = flag.String("config", "configs/apigateway.yaml", "配置文件路径")
)

func main() {
	flag.Parse()

	// 加载配置文件
	log.Printf("加载配置文件: %s", *configFile)
	// TODO: 加载配置文件

	// 初始化路由
	router := mux.NewRouter()

	// 注册路由
	router.HandleFunc("/api/v1/files", uploadFile).Methods("POST")
	router.HandleFunc("/api/v1/files/{id}", downloadFile).Methods("GET")
	router.HandleFunc("/api/v1/files/{id}", deleteFile).Methods("DELETE")
	router.HandleFunc("/api/v1/files/{id}/status", getFileStatus).Methods("GET")

	// 健康检查
	router.HandleFunc("/health", healthCheck).Methods("GET")

	// 设置服务器
	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 启动服务器
	go func() {
		log.Printf("API网关启动在端口: %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("无法启动服务器: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器强制关闭: %v", err)
	}

	log.Println("服务器已关闭")
}
