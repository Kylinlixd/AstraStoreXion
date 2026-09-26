package storage

import (
	"context"
	"io"
)

// StorageNode 存储节点接口
type StorageNode interface {
	// 写入数据块
	WriteChunk(ctx context.Context, chunkID string, data []byte) error

	// 读取数据块
	ReadChunk(ctx context.Context, chunkID string) ([]byte, error)

	// 删除数据块
	DeleteChunk(ctx context.Context, chunkID string) error

	// 获取节点状态
	GetStatus(ctx context.Context) (*NodeStatus, error)

	// 开始心跳检测
	StartHeartbeat(ctx context.Context) error

	// 停止心跳检测
	StopHeartbeat() error
}

// NodeStatus 节点状态
type NodeStatus struct {
	NodeID    string  `json:"node_id"`
	Address   string  `json:"address"`
	DiskUsage float64 `json:"disk_usage"` // 0-100 百分比
	IsLeader  bool    `json:"is_leader"`
	IsHealth  bool    `json:"is_health"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	NodeID       string `json:"node_id"`
	Address      string `json:"address"`
	DiskQuota    int64  `json:"disk_quota"`
	DataPath     string `json:"data_path"`
	ChunkSize    int    `json:"chunk_size"`    // 默认 4MB
	HeartbeatTTL int    `json:"heartbeat_ttl"` // 秒，默认 3 秒
}

// ChunkReader 块数据读取器
type ChunkReader interface {
	io.ReadCloser
	ChunkID() string
	Size() int64
}

// ChunkWriter 块数据写入器
type ChunkWriter interface {
	io.WriteCloser
	ChunkID() string
	Commit() error
	Abort() error
}
