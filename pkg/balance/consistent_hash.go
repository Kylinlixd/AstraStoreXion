package balance

import (
	"context"
	"errors"
	"hash/crc32"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ConsistentHashBalancer 一致性哈希实现的负载均衡器
type ConsistentHashBalancer struct {
	config     BalanceConfig
	ring       map[uint32]string // 哈希环
	sortedKeys []uint32          // 排序的哈希键
	nodes      map[string]*ShardStatus
	replicas   int // 虚拟节点数量
	mutex      sync.RWMutex
	isRunning  bool
	ctx        context.Context
	cancel     context.CancelFunc
	// 热点文件访问统计
	fileAccess     map[string]*fileAccessInfo
	fileAccessLock sync.RWMutex
}

// NewConsistentHashBalancer 创建一致性哈希负载均衡器
func NewConsistentHashBalancer(config BalanceConfig, replicas int) Balancer {
	return &ConsistentHashBalancer{
		config:     config,
		ring:       make(map[uint32]string),
		nodes:      make(map[string]*ShardStatus),
		replicas:   replicas,
		fileAccess: make(map[string]*fileAccessInfo),
	}
}

// Start 启动数据平衡
func (b *ConsistentHashBalancer) Start(ctx context.Context) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.isRunning {
		return errors.New("负载均衡器已启动")
	}

	b.ctx, b.cancel = context.WithCancel(ctx)
	b.isRunning = true

	go b.balanceRoutine()

	return nil
}

// Stop 停止数据平衡
func (b *ConsistentHashBalancer) Stop() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.isRunning {
		return errors.New("负载均衡器未启动")
	}

	b.cancel()
	b.isRunning = false

	return nil
}

// GetShardStatus 获取节点分片状态
func (b *ConsistentHashBalancer) GetShardStatus(ctx context.Context) (map[string]*ShardStatus, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	result := make(map[string]*ShardStatus, len(b.nodes))
	for nodeID, status := range b.nodes {
		result[nodeID] = &ShardStatus{
			NodeID:       status.NodeID,
			ShardCount:   status.ShardCount,
			DiskUsage:    status.DiskUsage,
			LoadFactor:   status.LoadFactor,
			IsOverloaded: status.IsOverloaded,
		}
	}

	return result, nil
}

// ForceRebalance 强制重平衡
func (b *ConsistentHashBalancer) ForceRebalance(ctx context.Context) error {
	return b.rebalance()
}

// MigrateShard 迁移分片
func (b *ConsistentHashBalancer) MigrateShard(ctx context.Context, shardID string, sourceNodeID, targetNodeID string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// 检查源节点和目标节点是否存在
	if _, exists := b.nodes[sourceNodeID]; !exists {
		return errors.New("源节点不存在")
	}

	if _, exists := b.nodes[targetNodeID]; !exists {
		return errors.New("目标节点不存在")
	}

	// TODO: 实现实际的数据迁移逻辑
	// 这里需要调用存储服务的API来迁移分片数据

	// 更新分片计数
	b.nodes[sourceNodeID].ShardCount--
	b.nodes[targetNodeID].ShardCount++

	return nil
}

// GetHotFiles 获取热点文件列表
func (b *ConsistentHashBalancer) GetHotFiles(ctx context.Context, limit int) ([]*HotFile, error) {
	b.fileAccessLock.RLock()
	defer b.fileAccessLock.RUnlock()

	if limit <= 0 {
		limit = 10 // 默认限制
	}

	// 创建热点文件列表
	hotFiles := make([]*HotFile, 0, len(b.fileAccess))

	// 筛选超过阈值的文件
	for fileID, info := range b.fileAccess {
		if info.accessCount >= b.config.HotFileThreshold {
			hotFiles = append(hotFiles, &HotFile{
				FileID:      fileID,
				AccessCount: info.accessCount,
				LastAccess:  info.lastAccess.Unix(),
			})
		}
	}

	// 按访问次数降序排序
	sort.Slice(hotFiles, func(i, j int) bool {
		return hotFiles[i].AccessCount > hotFiles[j].AccessCount
	})

	// 限制返回数量
	if len(hotFiles) > limit {
		hotFiles = hotFiles[:limit]
	}

	return hotFiles, nil
}

// AddNode 添加节点
func (b *ConsistentHashBalancer) AddNode(nodeID string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// 如果节点已存在，直接返回
	if _, exists := b.nodes[nodeID]; exists {
		return
	}

	// 添加节点状态
	b.nodes[nodeID] = &ShardStatus{
		NodeID:       nodeID,
		ShardCount:   0,
		DiskUsage:    0,
		LoadFactor:   0,
		IsOverloaded: false,
	}

	// 添加虚拟节点到哈希环
	for i := 0; i < b.replicas; i++ {
		key := b.hashKey(nodeID + strconv.Itoa(i))
		b.ring[key] = nodeID
		b.sortedKeys = append(b.sortedKeys, key)
	}

	// 重新排序哈希环
	sort.Slice(b.sortedKeys, func(i, j int) bool {
		return b.sortedKeys[i] < b.sortedKeys[j]
	})
}

// RemoveNode 移除节点
func (b *ConsistentHashBalancer) RemoveNode(nodeID string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// 如果节点不存在，直接返回
	if _, exists := b.nodes[nodeID]; !exists {
		return
	}

	// 从节点列表中移除
	delete(b.nodes, nodeID)

	// 从哈希环中移除所有虚拟节点
	var newKeys []uint32
	for i := 0; i < b.replicas; i++ {
		key := b.hashKey(nodeID + strconv.Itoa(i))
		delete(b.ring, key)
	}

	// 重建排序的哈希键
	for key := range b.ring {
		newKeys = append(newKeys, key)
	}
	sort.Slice(newKeys, func(i, j int) bool {
		return newKeys[i] < newKeys[j]
	})
	b.sortedKeys = newKeys
}

// GetNode 根据键获取节点
func (b *ConsistentHashBalancer) GetNode(key string) (string, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.ring) == 0 {
		return "", errors.New("哈希环为空")
	}

	// 计算哈希值
	hash := b.hashKey(key)

	// 找到第一个大于等于哈希值的节点
	idx := sort.Search(len(b.sortedKeys), func(i int) bool {
		return b.sortedKeys[i] >= hash
	})

	// 如果找不到，则返回第一个节点
	if idx == len(b.sortedKeys) {
		idx = 0
	}

	return b.ring[b.sortedKeys[idx]], nil
}

// UpdateNodeStatus 更新节点状态
func (b *ConsistentHashBalancer) UpdateNodeStatus(nodeID string, status *ShardStatus) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if _, exists := b.nodes[nodeID]; !exists {
		return
	}

	b.nodes[nodeID] = status
}

// 计算哈希键
func (b *ConsistentHashBalancer) hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// 平衡例程
func (b *ConsistentHashBalancer) balanceRoutine() {
	// 如果配置了自动平衡，定期检查和执行平衡
	if b.config.EnableAutoBalance {
		ticker := time.NewTicker(time.Duration(b.config.CheckInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.checkAndBalance()
			case <-b.ctx.Done():
				return
			}
		}
	}
}

// 检查并平衡
func (b *ConsistentHashBalancer) checkAndBalance() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	// 检查是否有过载的节点
	hasOverloaded := false
	for _, status := range b.nodes {
		if status.DiskUsage > b.config.DiskUsageThreshold ||
			status.LoadFactor > b.config.LoadFactorThreshold {
			status.IsOverloaded = true
			hasOverloaded = true
		} else {
			status.IsOverloaded = false
		}
	}

	// 如果有过载节点，进行平衡
	if hasOverloaded {
		return b.rebalance()
	}

	return nil
}

// 重平衡
func (b *ConsistentHashBalancer) rebalance() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.nodes) < 2 {
		return errors.New("节点数量不足，无法平衡")
	}

	// 按负载排序节点
	type nodeLoad struct {
		nodeID     string
		loadFactor float64
	}

	var loads []nodeLoad
	for nodeID, status := range b.nodes {
		loads = append(loads, nodeLoad{
			nodeID:     nodeID,
			loadFactor: status.LoadFactor,
		})
	}

	// 按负载因子降序排序
	sort.Slice(loads, func(i, j int) bool {
		return loads[i].loadFactor > loads[j].loadFactor
	})

	// 找到负载最高和最低的节点
	highNode := loads[0]
	lowNode := loads[len(loads)-1]

	// 如果负载差异不大，不需要平衡
	if highNode.loadFactor-lowNode.loadFactor < 0.3 {
		return nil
	}

	// TODO: 实现具体的分片迁移逻辑
	// 从高负载节点迁移分片到低负载节点

	return nil
}

// fileAccessInfo 文件访问信息
type fileAccessInfo struct {
	accessCount int64     // 访问次数
	lastAccess  time.Time // 最后访问时间
}

// TrackFileAccess 记录文件访问
func (b *ConsistentHashBalancer) TrackFileAccess(fileID string) {
	b.fileAccessLock.Lock()
	defer b.fileAccessLock.Unlock()

	info, exists := b.fileAccess[fileID]
	if !exists {
		info = &fileAccessInfo{
			accessCount: 0,
			lastAccess:  time.Now(),
		}
		b.fileAccess[fileID] = info
	}

	info.accessCount++
	info.lastAccess = time.Now()
}
