package metadata

import (
	"fmt"
	"sync"
	"time"
)

// FileMetadata 文件元数据
type FileMetadata struct {
	FileID    string
	ShardKey  uint32
	Chunks    []ChunkInfo
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChunkInfo 块信息
type ChunkInfo struct {
	ChunkID string
	NodeID  string
	Offset  int64
	Size    int64
}

// MetadataService 元数据服务接口
type MetadataService interface {
	// 创建文件元数据
	CreateMetadata(fileID string, shardKey uint32, chunks []ChunkInfo) error

	// 获取文件元数据
	GetMetadata(fileID string) (*FileMetadata, error)

	// 更新文件元数据
	UpdateMetadata(fileID string, chunks []ChunkInfo) error

	// 删除文件元数据
	DeleteMetadata(fileID string) error

	// 列出文件元数据
	ListMetadata(limit, offset int) ([]*FileMetadata, error)

	// 按节点ID列出文件元数据
	ListMetadataByNodeID(nodeID string, limit, offset int) ([]*FileMetadata, error)
}

// InMemoryMetadataService 内存实现的元数据服务
type InMemoryMetadataService struct {
	mutex sync.RWMutex
	data  map[string]*FileMetadata
}

// NewInMemoryMetadataService 创建一个新的内存元数据服务实例
func NewInMemoryMetadataService() MetadataService {
	return &InMemoryMetadataService{
		data: make(map[string]*FileMetadata),
	}
}

// CreateMetadata 创建文件元数据
func (s *InMemoryMetadataService) CreateMetadata(fileID string, shardKey uint32, chunks []ChunkInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.data[fileID]; exists {
		return fmt.Errorf("文件ID %s 已存在", fileID)
	}

	now := time.Now()
	s.data[fileID] = &FileMetadata{
		FileID:    fileID,
		ShardKey:  shardKey,
		Chunks:    chunks,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return nil
}

// GetMetadata 获取文件元数据
func (s *InMemoryMetadataService) GetMetadata(fileID string) (*FileMetadata, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	meta, exists := s.data[fileID]
	if !exists {
		return nil, fmt.Errorf("文件ID %s 不存在", fileID)
	}

	return meta, nil
}

// UpdateMetadata 更新文件元数据
func (s *InMemoryMetadataService) UpdateMetadata(fileID string, chunks []ChunkInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	meta, exists := s.data[fileID]
	if !exists {
		return fmt.Errorf("文件ID %s 不存在", fileID)
	}

	meta.Chunks = chunks
	meta.UpdatedAt = time.Now()

	return nil
}

// DeleteMetadata 删除文件元数据
func (s *InMemoryMetadataService) DeleteMetadata(fileID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.data[fileID]; !exists {
		return fmt.Errorf("文件ID %s 不存在", fileID)
	}

	delete(s.data, fileID)
	return nil
}

// ListMetadata 列出文件元数据
func (s *InMemoryMetadataService) ListMetadata(limit, offset int) ([]*FileMetadata, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if offset < 0 {
		offset = 0
	}

	if limit <= 0 {
		limit = 10 // 默认限制
	}

	var result []*FileMetadata
	var idx int

	for _, meta := range s.data {
		if idx >= offset && len(result) < limit {
			result = append(result, meta)
		}
		idx++
	}

	return result, nil
}

// ListMetadataByNodeID 按节点ID列出文件元数据
func (s *InMemoryMetadataService) ListMetadataByNodeID(nodeID string, limit, offset int) ([]*FileMetadata, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if offset < 0 {
		offset = 0
	}

	if limit <= 0 {
		limit = 10 // 默认限制
	}

	var result []*FileMetadata
	var idx int

	for _, meta := range s.data {
		// 检查是否有任何块属于该节点
		hasNode := false
		for _, chunk := range meta.Chunks {
			if chunk.NodeID == nodeID {
				hasNode = true
				break
			}
		}

		if hasNode {
			if idx >= offset && len(result) < limit {
				result = append(result, meta)
			}
			idx++
		}
	}

	return result, nil
}
