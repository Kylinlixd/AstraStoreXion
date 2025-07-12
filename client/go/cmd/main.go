package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	client "github.com/astrastore/astrastore-xion/client/go"
)

var (
	configFile = flag.String("config", "xion-client.yaml", "客户端配置文件")
	apiGateway = flag.String("api", "http://localhost:8080", "API网关地址")
	timeout    = flag.Duration("timeout", 30*time.Second, "请求超时时间")
)

func main() {
	// 解析命令行参数
	flag.Parse()

	// 检查命令
	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// 创建客户端配置
	config := &client.Config{
		APIGateway: *apiGateway,
		Timeout:    *timeout,
		MaxRetries: 3,
	}

	// 创建客户端
	c, err := client.NewClient(config)
	if err != nil {
		fmt.Printf("创建客户端失败: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// 处理命令
	cmd := args[0]
	switch cmd {
	case "upload":
		if len(args) < 2 {
			fmt.Println("缺少文件路径")
			os.Exit(1)
		}
		uploadFile(ctx, c, args[1])
	case "download":
		if len(args) < 3 {
			fmt.Println("用法: download <file-id> <output-path>")
			os.Exit(1)
		}
		downloadFile(ctx, c, args[1], args[2])
	case "status":
		if len(args) < 2 {
			fmt.Println("缺少文件ID")
			os.Exit(1)
		}
		getFileStatus(ctx, c, args[1])
	case "delete":
		if len(args) < 2 {
			fmt.Println("缺少文件ID")
			os.Exit(1)
		}
		deleteFile(ctx, c, args[1])
	default:
		fmt.Printf("未知命令: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// 打印使用说明
func printUsage() {
	fmt.Println("使用方法:")
	fmt.Println("  xion upload <file-path> - 上传文件")
	fmt.Println("  xion download <file-id> <output-path> - 下载文件")
	fmt.Println("  xion status <file-id> - 获取文件状态")
	fmt.Println("  xion delete <file-id> - 删除文件")
	fmt.Println("\n选项:")
	flag.PrintDefaults()
}

// 上传文件
func uploadFile(ctx context.Context, c client.Client, filePath string) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("打开文件失败: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// 获取文件名
	filename := filepath.Base(filePath)

	// 准备元数据
	metadata := map[string]string{
		"created_by": os.Getenv("USER"),
	}

	// 上传文件
	fmt.Printf("正在上传文件 %s...\n", filename)
	resp, err := c.UploadFile(ctx, file, filename, metadata)
	if err != nil {
		fmt.Printf("上传失败: %v\n", err)
		os.Exit(1)
	}

	// 打印结果
	fmt.Printf("上传成功! 文件ID: %s\n", resp.FileID)
}

// 下载文件
func downloadFile(ctx context.Context, c client.Client, fileID, outputPath string) {
	// 创建输出文件
	output, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("创建输出文件失败: %v\n", err)
		os.Exit(1)
	}
	defer output.Close()

	// 下载文件
	fmt.Printf("正在下载文件 %s 到 %s...\n", fileID, outputPath)
	err = c.DownloadFile(ctx, fileID, output)
	if err != nil {
		fmt.Printf("下载失败: %v\n", err)
		os.Exit(1)
	}

	// 打印结果
	fmt.Println("下载成功!")
}

// 获取文件状态
func getFileStatus(ctx context.Context, c client.Client, fileID string) {
	// 获取文件状态
	fmt.Printf("正在获取文件 %s 的状态...\n", fileID)
	status, err := c.GetFileStatus(ctx, fileID)
	if err != nil {
		fmt.Printf("获取状态失败: %v\n", err)
		os.Exit(1)
	}

	// 打印结果
	fmt.Printf("文件ID: %s\n", status.FileID)
	fmt.Printf("文件名: %s\n", status.Filename)
	fmt.Printf("大小: %d 字节\n", status.Size)
	fmt.Printf("状态: %s\n", status.Status)
	fmt.Println("元数据:")
	for k, v := range status.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

// 删除文件
func deleteFile(ctx context.Context, c client.Client, fileID string) {
	// 删除文件
	fmt.Printf("正在删除文件 %s...\n", fileID)
	resp, err := c.DeleteFile(ctx, fileID)
	if err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}

	// 打印结果
	if resp.Success {
		fmt.Println("删除成功!")
	} else {
		fmt.Printf("删除失败: %s\n", resp.Message)
	}
}
