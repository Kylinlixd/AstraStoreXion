package unit

import (
	"fmt"
	"testing"

	"github.com/astrastore/astrastore-xion/pkg/metadata"
	"github.com/stretchr/testify/assert"
)

func TestFileMetadataOperations(t *testing.T) {
	// 创建内存元数据服务实例
	svc := metadata.NewInMemoryMetadataService()

	// 准备测试数据
	fileID := "test-file-1"
	shardKey := uint32(12345)
	chunks := []metadata.ChunkInfo{
		{
			ChunkID: "chunk-1",
			NodeID:  "node-1",
			Offset:  0,
			Size:    1024,
		},
		{
			ChunkID: "chunk-2",
			NodeID:  "node-2",
			Offset:  1024,
			Size:    1024,
		},
	}

	// 测试创建元数据
	err := svc.CreateMetadata(fileID, shardKey, chunks)
	assert.NoError(t, err)

	// 测试获取元数据
	meta, err := svc.GetMetadata(fileID)
	assert.NoError(t, err)
	assert.Equal(t, fileID, meta.FileID)
	assert.Equal(t, shardKey, meta.ShardKey)
	assert.Len(t, meta.Chunks, 2)
	assert.Equal(t, "chunk-1", meta.Chunks[0].ChunkID)
	assert.Equal(t, "node-1", meta.Chunks[0].NodeID)

	// 测试更新元数据
	newChunks := append(chunks, metadata.ChunkInfo{
		ChunkID: "chunk-3",
		NodeID:  "node-3",
		Offset:  2048,
		Size:    1024,
	})
	err = svc.UpdateMetadata(fileID, newChunks)
	assert.NoError(t, err)

	// 验证更新后的元数据
	meta, err = svc.GetMetadata(fileID)
	assert.NoError(t, err)
	assert.Len(t, meta.Chunks, 3)
	assert.Equal(t, "chunk-3", meta.Chunks[2].ChunkID)

	// 测试删除元数据
	err = svc.DeleteMetadata(fileID)
	assert.NoError(t, err)

	// 验证删除后无法获取元数据
	_, err = svc.GetMetadata(fileID)
	assert.Error(t, err)
}

func TestListMetadata(t *testing.T) {
	// 创建内存元数据服务实例
	svc := metadata.NewInMemoryMetadataService()

	// 准备多个测试数据
	for i := 1; i <= 5; i++ {
		fileID := fmt.Sprintf("test-file-%d", i)
		shardKey := uint32(i * 1000)
		chunks := []metadata.ChunkInfo{
			{
				ChunkID: fmt.Sprintf("chunk-%d", i),
				NodeID:  fmt.Sprintf("node-%d", i%3),
				Offset:  0,
				Size:    1024 * int64(i),
			},
		}
		err := svc.CreateMetadata(fileID, shardKey, chunks)
		assert.NoError(t, err)
	}

	// 测试列出所有元数据
	metas, err := svc.ListMetadata(10, 0)
	assert.NoError(t, err)
	assert.Len(t, metas, 5)

	// 测试分页
	metas, err = svc.ListMetadata(2, 0)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)

	metas, err = svc.ListMetadata(2, 2)
	assert.NoError(t, err)
	assert.Len(t, metas, 2)

	metas, err = svc.ListMetadata(2, 4)
	assert.NoError(t, err)
	assert.Len(t, metas, 1)

	// 测试按节点ID过滤
	metas, err = svc.ListMetadataByNodeID("node-1", 10, 0)
	assert.NoError(t, err)
	// 节点ID为1的应该有2个文件 (当i%3=1时，即i=1和i=4)
	assert.Len(t, metas, 2)
}
