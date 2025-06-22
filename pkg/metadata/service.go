package metadata

import (
	"context"
	"time"
)

// FileMetadata 文件元数据结构
type FileMetadata struct {
	FileID    string      `json:"file_id"`
	ShardKey  uint32      `json:"shard_key"`
	Chunks    []ChunkInfo `json:"chunks"`
	CreatedAt time.Time   `json:"created_at"`
}

// ChunkInfo 块信息
type ChunkInfo struct {
	ChunkID string `json:"chunk_id"`
	NodeID  string `json:"node_id"`
	Offset  int64  `json:"offset"`
	Size    int64  `json:"size"`
}

// MetadataService 元数据服务接口
type MetadataService interface {
	// 创建元数据
	CreateMetadata(ctx context.Context, fileID string, shardKey uint32, chunks []ChunkInfo) error
	
	// 获取元数据
	GetMetadata(ctx context.Context, fileID string) (*FileMetadata, error)
	
	// 更新元数据
	UpdateMetadata(ctx context.Context, fileID string, metadata *FileMetadata) error
	
	// 删除元数据
	DeleteMetadata(ctx context.Context, fileID string) error
} 