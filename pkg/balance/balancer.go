package balance

import (
	"context"
)

// Balancer 数据平衡器接口
type Balancer interface {
	// 启动数据平衡
	Start(ctx context.Context) error

	// 停止数据平衡
	Stop() error

	// 获取节点分片状态
	GetShardStatus(ctx context.Context) (map[string]*ShardStatus, error)

	// 强制重平衡
	ForceRebalance(ctx context.Context) error

	// 迁移分片
	MigrateShard(ctx context.Context, shardID string, sourceNodeID, targetNodeID string) error

	// 获取热点文件列表
	GetHotFiles(ctx context.Context, limit int) ([]*HotFile, error)
}

// ShardStatus 分片状态
type ShardStatus struct {
	NodeID       string  `json:"node_id"`
	ShardCount   int     `json:"shard_count"`
	DiskUsage    float64 `json:"disk_usage"`  // 0-100 百分比
	LoadFactor   float64 `json:"load_factor"` // 负载因子，越高表示负载越重
	IsOverloaded bool    `json:"is_overloaded"`
}

// HotFile 热点文件
type HotFile struct {
	FileID      string `json:"file_id"`
	AccessCount int64  `json:"access_count"` // 最近一段时间的访问次数
	LastAccess  int64  `json:"last_access"`  // 最后访问时间戳
}

// BalanceConfig 平衡配置
type BalanceConfig struct {
	CheckInterval       int     `json:"check_interval"`        // 检查间隔（秒）
	DiskUsageThreshold  float64 `json:"disk_usage_threshold"`  // 磁盘使用率阈值
	LoadFactorThreshold float64 `json:"load_factor_threshold"` // 负载因子阈值
	HotFileThreshold    int64   `json:"hot_file_threshold"`    // 热点文件访问次数阈值
	EnableAutoBalance   bool    `json:"enable_auto_balance"`   // 是否启用自动平衡
}
