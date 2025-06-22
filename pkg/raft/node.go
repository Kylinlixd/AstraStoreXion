package raft

import (
	"context"
)

// RaftNode Raft节点接口
type RaftNode interface {
	// 启动节点
	Start(ctx context.Context) error
	
	// 停止节点
	Stop() error
	
	// 应用命令（写操作）
	Apply(ctx context.Context, cmd []byte, timeout int) (interface{}, error)
	
	// 添加新节点
	AddPeer(ctx context.Context, nodeID, address string) error
	
	// 移除节点
	RemovePeer(ctx context.Context, nodeID string) error
	
	// 是否是Leader
	IsLeader() bool
	
	// 获取当前Leader
	GetLeader() string
	
	// 获取节点状态
	GetState() RaftState
}

// RaftState Raft状态
type RaftState struct {
	NodeID      string   `json:"node_id"`
	Role        string   `json:"role"`        // "leader", "follower", "candidate"
	Term        uint64   `json:"term"`        // 当前任期
	LastIndex   uint64   `json:"last_index"`  // 最后日志索引
	Peers       []string `json:"peers"`       // 集群中的其他节点
	CommitIndex uint64   `json:"commit_index"` // 已提交的最高日志条目的索引
}

// RaftConfig Raft配置
type RaftConfig struct {
	NodeID          string   `json:"node_id"`
	Address         string   `json:"address"`
	DataDir         string   `json:"data_dir"`
	Peers           []string `json:"peers"`
	HeartbeatTimeout int      `json:"heartbeat_timeout"` // 毫秒
	ElectionTimeout  int      `json:"election_timeout"`  // 毫秒
	PreVote         bool     `json:"pre_vote"`          // 是否启用Pre-Vote机制
} 