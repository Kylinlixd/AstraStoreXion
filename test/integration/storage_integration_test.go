package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"bytes"

	"github.com/astrastore/astrastore-xion/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStorageNode(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "storage-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建存储节点配置
	config := storage.StorageConfig{
		NodeID:       "test-node-1",
		Address:      "localhost:8001",
		DiskQuota:    1024 * 1024 * 100, // 100MB
		DataPath:     tempDir,
		ChunkSize:    1024 * 1024, // 1MB
		HeartbeatTTL: 1,
	}

	// 创建存储节点
	node, err := storage.NewLocalStorageNode(config)
	require.NoError(t, err)

	// 创建测试数据
	testData := make([]byte, 1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	ctx := context.Background()

	// 写入块
	err = node.WriteChunk(ctx, "test-chunk-1", testData)
	assert.NoError(t, err)

	// 读取块
	readData, err := node.ReadChunk(ctx, "test-chunk-1")
	assert.NoError(t, err)
	assert.Equal(t, testData, readData)

	// 获取节点状态
	status, err := node.GetStatus(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "test-node-1", status.NodeID)
	assert.Equal(t, "localhost:8001", status.Address)
	assert.True(t, status.DiskUsage >= 0)
	assert.True(t, status.IsHealth)

	// 启动心跳
	err = node.StartHeartbeat(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(2 * time.Second)

	// 停止心跳
	err = node.StopHeartbeat()
	assert.NoError(t, err)

	// 删除块
	err = node.DeleteChunk(ctx, "test-chunk-1")
	assert.NoError(t, err)

	// 尝试读取已删除的块
	_, err = node.ReadChunk(ctx, "test-chunk-1")
	assert.Error(t, err)
}

func TestStorageNodeConcurrency(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "storage-concurrency-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 创建存储节点配置
	config := storage.StorageConfig{
		NodeID:       "test-node-2",
		Address:      "localhost:8002",
		DiskQuota:    1024 * 1024 * 100, // 100MB
		DataPath:     tempDir,
		ChunkSize:    1024 * 1024, // 1MB
		HeartbeatTTL: 1,
	}

	// 创建存储节点
	node, err := storage.NewLocalStorageNode(config)
	require.NoError(t, err)

	ctx := context.Background()

	// 并发写入和读取
	const numChunks = 10
	const numGoroutines = 5

	// 创建错误通道
	errCh := make(chan error, numGoroutines*numChunks)

	// 并发写入
	for g := 0; g < numGoroutines; g++ {
		go func(gid int) {
			for i := 0; i < numChunks; i++ {
				chunkID := fmt.Sprintf("test-chunk-%d-%d", gid, i)

				// 创建测试数据
				testData := make([]byte, 1024)
				for j := range testData {
					testData[j] = byte((gid*numChunks + i + j) % 256)
				}

				// 写入块
				if err := node.WriteChunk(ctx, chunkID, testData); err != nil {
					errCh <- err
					return
				}

				// 读取块
				readData, err := node.ReadChunk(ctx, chunkID)
				if err != nil {
					errCh <- err
					return
				}

				// 验证数据
				if !bytes.Equal(testData, readData) {
					errCh <- fmt.Errorf("数据不匹配: %s", chunkID)
					return
				}
			}
		}(g)
	}

	// 等待所有goroutine完成
	time.Sleep(2 * time.Second)

	// 检查错误
	close(errCh)
	for err := range errCh {
		assert.NoError(t, err)
	}
}
