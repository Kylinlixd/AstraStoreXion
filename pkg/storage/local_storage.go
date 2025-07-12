package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// LocalStorageNode 本地存储节点实现
type LocalStorageNode struct {
	config       StorageConfig
	isRunning    bool
	heartbeatCtx context.Context
	cancelFunc   context.CancelFunc
	bufferPool   sync.Pool
	mu           sync.RWMutex
}

// NewLocalStorageNode 创建一个新的本地存储节点
func NewLocalStorageNode(config StorageConfig) (StorageNode, error) {
	// 验证配置
	if config.NodeID == "" {
		return nil, errors.New("节点ID不能为空")
	}
	if config.DataPath == "" {
		return nil, errors.New("数据路径不能为空")
	}

	// 确保数据目录存在
	if err := os.MkdirAll(config.DataPath, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %v", err)
	}

	// 设置默认值
	if config.ChunkSize == 0 {
		config.ChunkSize = 4 * 1024 * 1024 // 默认4MB
	}
	if config.HeartbeatTTL == 0 {
		config.HeartbeatTTL = 3 // 默认3秒
	}

	return &LocalStorageNode{
		config: config,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, config.ChunkSize)
			},
		},
	}, nil
}

// WriteChunk 写入数据块
func (n *LocalStorageNode) WriteChunk(ctx context.Context, chunkID string, data []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 构造块文件路径
	chunkPath := filepath.Join(n.config.DataPath, chunkID)

	// 创建父目录
	dir := filepath.Dir(chunkPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 创建临时文件
	tmpPath := chunkPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer file.Close()

	// 写入数据
	if _, err := file.Write(data); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("写入数据失败: %v", err)
	}

	// 确保数据写入磁盘
	if err := file.Sync(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("同步数据失败: %v", err)
	}

	// 重命名临时文件
	if err := os.Rename(tmpPath, chunkPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	return nil
}

// ReadChunk 读取数据块
func (n *LocalStorageNode) ReadChunk(ctx context.Context, chunkID string) ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// 构造块文件路径
	chunkPath := filepath.Join(n.config.DataPath, chunkID)

	// 打开文件
	file, err := os.Open(chunkPath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 获取文件大小
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %v", err)
	}

	// 读取数据
	data := make([]byte, fileInfo.Size())
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, fmt.Errorf("读取数据失败: %v", err)
	}

	return data, nil
}

// DeleteChunk 删除数据块
func (n *LocalStorageNode) DeleteChunk(ctx context.Context, chunkID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 构造块文件路径
	chunkPath := filepath.Join(n.config.DataPath, chunkID)

	// 删除文件
	if err := os.Remove(chunkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %v", err)
	}

	return nil
}

// GetStatus 获取节点状态
func (n *LocalStorageNode) GetStatus(ctx context.Context) (*NodeStatus, error) {
	// 获取磁盘使用情况
	diskUsage, err := n.getDiskUsage()
	if err != nil {
		return nil, fmt.Errorf("获取磁盘使用率失败: %v", err)
	}

	return &NodeStatus{
		NodeID:    n.config.NodeID,
		Address:   n.config.Address,
		DiskUsage: diskUsage,
		IsLeader:  false, // 需要结合Raft状态
		IsHealth:  true,
	}, nil
}

// getDiskUsage 获取磁盘使用率
func (n *LocalStorageNode) getDiskUsage() (float64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(n.config.DataPath, &stat)
	if err != nil {
		return 0, err
	}

	// 计算总空间和可用空间
	totalBlocks := stat.Blocks
	freeBlocks := stat.Bfree

	// 如果有设置磁盘配额，以较小的值为准
	if n.config.DiskQuota > 0 {
		blockSize := uint64(stat.Bsize)
		quotaBlocks := uint64(n.config.DiskQuota) / blockSize
		if quotaBlocks < totalBlocks {
			totalBlocks = quotaBlocks
			// 重新计算可用空间，考虑已使用的空间
			usedBlocks := n.getUsedBlocks()
			if usedBlocks < quotaBlocks {
				freeBlocks = quotaBlocks - usedBlocks
			} else {
				freeBlocks = 0
			}
		}
	}

	// 计算使用率
	if totalBlocks == 0 {
		return 0, nil
	}

	usedBlocks := totalBlocks - freeBlocks
	usageRatio := float64(usedBlocks) / float64(totalBlocks)

	// 转为百分比
	return usageRatio * 100, nil
}

// getUsedBlocks 获取已使用的块数
func (n *LocalStorageNode) getUsedBlocks() uint64 {
	var totalSize uint64

	// 遍历目录获取所有文件大小
	filepath.Walk(n.config.DataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误
		}
		if !info.IsDir() {
			totalSize += uint64(info.Size())
		}
		return nil
	})

	// 获取块大小
	var stat syscall.Statfs_t
	if err := syscall.Statfs(n.config.DataPath, &stat); err != nil {
		return 0
	}
	blockSize := uint64(stat.Bsize)

	// 计算块数
	if blockSize == 0 {
		return 0
	}
	return (totalSize + blockSize - 1) / blockSize // 向上取整
}

// StartHeartbeat 开始心跳检测
func (n *LocalStorageNode) StartHeartbeat(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isRunning {
		return errors.New("心跳已启动")
	}

	n.heartbeatCtx, n.cancelFunc = context.WithCancel(ctx)
	n.isRunning = true

	go func() {
		ticker := time.NewTicker(time.Duration(n.config.HeartbeatTTL) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 发送心跳
				status, err := n.GetStatus(context.Background())
				if err == nil {
					// 将状态发送到注册中心或元数据服务
					log.Printf("节点[%s]心跳: 磁盘使用率: %.2f%%", n.config.NodeID, status.DiskUsage)
				}
			case <-n.heartbeatCtx.Done():
				return
			}
		}
	}()

	return nil
}

// StopHeartbeat 停止心跳检测
func (n *LocalStorageNode) StopHeartbeat() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.isRunning {
		return errors.New("心跳未启动")
	}

	n.cancelFunc()
	n.isRunning = false

	return nil
}
