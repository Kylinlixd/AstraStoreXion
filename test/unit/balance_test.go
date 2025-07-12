package unit

import (
	"context"
	"testing"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/balance"
	"github.com/stretchr/testify/assert"
)

func TestConsistentHashBalancer(t *testing.T) {
	// 创建配置
	config := balance.BalanceConfig{
		CheckInterval:       5,
		DiskUsageThreshold:  80.0,
		LoadFactorThreshold: 0.8,
		HotFileThreshold:    1000,
		EnableAutoBalance:   true,
	}

	// 创建均衡器
	balancer := balance.NewConsistentHashBalancer(config, 10)

	// 测试启动和停止
	ctx := context.Background()
	err := balancer.Start(ctx)
	assert.NoError(t, err)

	// 获取初始状态 (应该为空)
	status, err := balancer.GetShardStatus(ctx)
	assert.NoError(t, err)
	assert.Empty(t, status)

	// 添加节点 (使用反射或类型断言访问非公开方法，实际项目中可能需要调整测试方式)
	if consistentHashBalancer, ok := balancer.(*balance.ConsistentHashBalancer); ok {
		consistentHashBalancer.AddNode("node-1")
		consistentHashBalancer.AddNode("node-2")
		consistentHashBalancer.AddNode("node-3")
	}

	// 再次获取状态
	status, err = balancer.GetShardStatus(ctx)
	assert.NoError(t, err)
	assert.Len(t, status, 3)

	// 验证节点是否存在
	_, exists := status["node-1"]
	assert.True(t, exists)
	_, exists = status["node-2"]
	assert.True(t, exists)
	_, exists = status["node-3"]
	assert.True(t, exists)

	// 测试节点选择
	if consistentHashBalancer, ok := balancer.(*balance.ConsistentHashBalancer); ok {
		// 测试相同的key总是映射到相同的节点
		node1, err := consistentHashBalancer.GetNode("test-key-1")
		assert.NoError(t, err)
		node2, err := consistentHashBalancer.GetNode("test-key-1")
		assert.NoError(t, err)
		assert.Equal(t, node1, node2)

		// 测试不同的key可能映射到不同的节点
		keys := []string{"key-1", "key-2", "key-3", "key-4", "key-5"}
		nodes := make(map[string]bool)

		for _, key := range keys {
			node, err := consistentHashBalancer.GetNode(key)
			assert.NoError(t, err)
			nodes[node] = true
		}

		// 确认至少有两个节点被使用
		assert.True(t, len(nodes) >= 2)
	}

	// 测试重平衡
	err = balancer.ForceRebalance(ctx)
	assert.NoError(t, err)

	// 停止均衡器
	err = balancer.Stop()
	assert.NoError(t, err)
}

func TestHotFilesDetection(t *testing.T) {
	// 创建配置
	config := balance.BalanceConfig{
		CheckInterval:       5,
		DiskUsageThreshold:  80.0,
		LoadFactorThreshold: 0.8,
		HotFileThreshold:    10, // 低阈值方便测试
		EnableAutoBalance:   true,
	}

	// 创建均衡器
	balancer := balance.NewConsistentHashBalancer(config, 10)

	// 启动均衡器
	ctx := context.Background()
	err := balancer.Start(ctx)
	assert.NoError(t, err)
	defer balancer.Stop()

	// 模拟记录文件访问
	if tracker, ok := balancer.(interface{ TrackFileAccess(fileID string) }); ok {
		// 访问文件多次
		for i := 0; i < 20; i++ {
			tracker.TrackFileAccess("hot-file-1")
		}

		// 访问其他文件较少次数
		tracker.TrackFileAccess("normal-file-1")
		tracker.TrackFileAccess("normal-file-1")
		tracker.TrackFileAccess("normal-file-2")
	}

	// 等待一段时间以确保统计已更新
	time.Sleep(100 * time.Millisecond)

	// 获取热点文件
	hotFiles, err := balancer.GetHotFiles(ctx, 10)
	assert.NoError(t, err)

	// 应该至少有一个热点文件
	if assert.NotEmpty(t, hotFiles) {
		// 第一个应该是访问次数最多的
		assert.Equal(t, "hot-file-1", hotFiles[0].FileID)
		assert.True(t, hotFiles[0].AccessCount >= 10)
	}
}
