package metadata

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type FileStatus string

const (
	FileStatusReady   FileStatus = "ready"
	FileStatusDeleted FileStatus = "deleted"
)

// FileMetadata 文件元数据
type FileMetadata struct {
	FileID      string
	Name        string
	ContentType string
	Size        int64
	Checksum    string
	OwnerID     string
	Status      FileStatus
	ShardKey    uint32
	Chunks      []ChunkInfo
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ChunkInfo 块信息
type ChunkInfo struct {
	ChunkID string
	NodeID  string
	Offset  int64
	Size    int64
}

type CreateFileMetadataInput struct {
	FileID      string
	Name        string
	ContentType string
	Size        int64
	Checksum    string
	OwnerID     string
	Status      FileStatus
	ShardKey    uint32
	Chunks      []ChunkInfo
}

// MetadataService 元数据服务接口
type MetadataService interface {
	// 创建完整文件元数据
	CreateFileMetadata(input CreateFileMetadataInput) error

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
	return s.CreateFileMetadata(CreateFileMetadataInput{
		FileID:   fileID,
		ShardKey: shardKey,
		Status:   FileStatusReady,
		Chunks:   chunks,
	})
}

// CreateFileMetadata 创建完整文件元数据
func (s *InMemoryMetadataService) CreateFileMetadata(input CreateFileMetadataInput) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.data[input.FileID]; exists {
		return fmt.Errorf("文件ID %s 已存在", input.FileID)
	}
	if input.Status == "" {
		input.Status = FileStatusReady
	}

	now := time.Now()
	s.data[input.FileID] = &FileMetadata{
		FileID:      input.FileID,
		Name:        input.Name,
		ContentType: input.ContentType,
		Size:        input.Size,
		Checksum:    input.Checksum,
		OwnerID:     input.OwnerID,
		Status:      input.Status,
		ShardKey:    input.ShardKey,
		Chunks:      cloneChunks(input.Chunks),
		CreatedAt:   now,
		UpdatedAt:   now,
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

	return cloneMetadata(meta), nil
}

// UpdateMetadata 更新文件元数据
func (s *InMemoryMetadataService) UpdateMetadata(fileID string, chunks []ChunkInfo) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	meta, exists := s.data[fileID]
	if !exists {
		return fmt.Errorf("文件ID %s 不存在", fileID)
	}

	meta.Chunks = cloneChunks(chunks)
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

	items := s.sortedMetadata()
	if offset >= len(items) {
		return []*FileMetadata{}, nil
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	return items[offset:end], nil
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

	var filtered []*FileMetadata

	for _, meta := range s.sortedMetadata() {
		// 检查是否有任何块属于该节点
		hasNode := false
		for _, chunk := range meta.Chunks {
			if chunk.NodeID == nodeID {
				hasNode = true
				break
			}
		}

		if hasNode {
			filtered = append(filtered, meta)
		}
	}

	if offset >= len(filtered) {
		return []*FileMetadata{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end], nil
}

func (s *InMemoryMetadataService) sortedMetadata() []*FileMetadata {
	items := make([]*FileMetadata, 0, len(s.data))
	for _, meta := range s.data {
		items = append(items, cloneMetadata(meta))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].FileID < items[j].FileID
	})

	return items
}

func cloneMetadata(meta *FileMetadata) *FileMetadata {
	if meta == nil {
		return nil
	}
	clone := *meta
	clone.Chunks = cloneChunks(meta.Chunks)
	return &clone
}

func cloneChunks(chunks []ChunkInfo) []ChunkInfo {
	if chunks == nil {
		return nil
	}
	clone := make([]ChunkInfo, len(chunks))
	copy(clone, chunks)
	return clone
}
