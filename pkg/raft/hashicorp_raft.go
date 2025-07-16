package raft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

// HashicorpRaftNode Hashicorp Raft实现
type HashicorpRaftNode struct {
	config    RaftConfig
	raft      *raft.Raft
	fsm       raft.FSM
	transport raft.Transport
	mutex     sync.RWMutex
	applying  sync.Mutex
}

// NewHashicorpRaftNode 创建Hashicorp Raft节点
func NewHashicorpRaftNode(config RaftConfig, fsm raft.FSM) (RaftNode, error) {
	// 验证配置
	if config.NodeID == "" {
		return nil, errors.New("节点ID不能为空")
	}
	if config.Address == "" {
		return nil, errors.New("地址不能为空")
	}
	if config.DataDir == "" {
		return nil, errors.New("数据目录不能为空")
	}

	// 创建数据目录
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %v", err)
	}

	// 设置默认值
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 1000 // 默认1秒
	}
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 1000 // 默认1秒
	}

	return &HashicorpRaftNode{
		config: config,
		fsm:    fsm,
	}, nil
}

// Start 启动节点
func (n *HashicorpRaftNode) Start(ctx context.Context) error {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.raft != nil {
		return errors.New("节点已启动")
	}

	// 创建Raft配置
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(n.config.NodeID)
	config.HeartbeatTimeout = time.Duration(n.config.HeartbeatTimeout) * time.Millisecond
	config.ElectionTimeout = time.Duration(n.config.ElectionTimeout) * time.Millisecond
	// 移除PreVote配置，新版本可能不支持
	// config.PreVote = n.config.PreVote

	// 创建日志存储
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(n.config.DataDir, "raft-log.bolt"))
	if err != nil {
		return fmt.Errorf("创建日志存储失败: %v", err)
	}

	// 创建稳定存储
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(n.config.DataDir, "raft-stable.bolt"))
	if err != nil {
		return fmt.Errorf("创建稳定存储失败: %v", err)
	}

	// 创建快照存储
	snapshotStore, err := raft.NewFileSnapshotStore(n.config.DataDir, 3, os.Stderr)
	if err != nil {
		return fmt.Errorf("创建快照存储失败: %v", err)
	}

	// 创建传输层
	addr, err := raft.NewTCPTransport(n.config.Address, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return fmt.Errorf("创建传输层失败: %v", err)
	}
	n.transport = addr

	// 创建Raft实例
	r, err := raft.NewRaft(config, n.fsm, logStore, stableStore, snapshotStore, n.transport)
	if err != nil {
		return fmt.Errorf("创建Raft实例失败: %v", err)
	}
	n.raft = r

	// 如果有初始节点，启动集群
	if len(n.config.Peers) > 0 && n.config.NodeID == n.config.Peers[0] {
		configuration := raft.Configuration{
			Servers: []raft.Server{},
		}

		// 添加所有节点
		for _, peer := range n.config.Peers {
			server := raft.Server{
				ID:      raft.ServerID(peer),
				Address: raft.ServerAddress(peer),
			}
			configuration.Servers = append(configuration.Servers, server)
		}

		// 初始化集群
		future := n.raft.BootstrapCluster(configuration)
		if err := future.Error(); err != nil && err != raft.ErrCantBootstrap {
			return fmt.Errorf("引导集群失败: %v", err)
		}
	}

	return nil
}

// Stop 停止节点
func (n *HashicorpRaftNode) Stop() error {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.raft == nil {
		return errors.New("节点未启动")
	}

	// 停止Raft实例
	future := n.raft.Shutdown()
	if err := future.Error(); err != nil {
		return fmt.Errorf("关闭Raft实例失败: %v", err)
	}

	// 关闭传输层
	if err := n.transport.(io.Closer).Close(); err != nil {
		return fmt.Errorf("关闭传输层失败: %v", err)
	}

	n.raft = nil
	return nil
}

// Apply 应用命令
func (n *HashicorpRaftNode) Apply(ctx context.Context, cmd []byte, timeout int) (interface{}, error) {
	n.mutex.RLock()
	if n.raft == nil {
		n.mutex.RUnlock()
		return nil, errors.New("节点未启动")
	}
	r := n.raft
	n.mutex.RUnlock()

	// 只有Leader可以应用命令
	if r.State() != raft.Leader {
		return nil, errors.New("不是Leader节点")
	}

	// 应用命令
	n.applying.Lock()
	defer n.applying.Unlock()

	// 设置超时
	if timeout <= 0 {
		timeout = 5000 // 默认5秒
	}
	timeoutDuration := time.Duration(timeout) * time.Millisecond

	// 应用日志
	future := r.Apply(cmd, timeoutDuration)
	if err := future.Error(); err != nil {
		return nil, fmt.Errorf("应用命令失败: %v", err)
	}

	// 返回结果
	return future.Response(), nil
}

// AddPeer 添加新节点
func (n *HashicorpRaftNode) AddPeer(ctx context.Context, nodeID, address string) error {
	n.mutex.RLock()
	if n.raft == nil {
		n.mutex.RUnlock()
		return errors.New("节点未启动")
	}
	r := n.raft
	n.mutex.RUnlock()

	// 只有Leader可以添加节点
	if r.State() != raft.Leader {
		return errors.New("不是Leader节点")
	}

	// 添加节点
	future := r.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("添加节点失败: %v", err)
	}

	return nil
}

// RemovePeer 移除节点
func (n *HashicorpRaftNode) RemovePeer(ctx context.Context, nodeID string) error {
	n.mutex.RLock()
	if n.raft == nil {
		n.mutex.RUnlock()
		return errors.New("节点未启动")
	}
	r := n.raft
	n.mutex.RUnlock()

	// 只有Leader可以移除节点
	if r.State() != raft.Leader {
		return errors.New("不是Leader节点")
	}

	// 移除节点
	future := r.RemoveServer(raft.ServerID(nodeID), 0, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("移除节点失败: %v", err)
	}

	return nil
}

// IsLeader 是否是Leader
func (n *HashicorpRaftNode) IsLeader() bool {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	if n.raft == nil {
		return false
	}

	return n.raft.State() == raft.Leader
}

// GetLeader 获取当前Leader
func (n *HashicorpRaftNode) GetLeader() string {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	if n.raft == nil {
		return ""
	}

	return string(n.raft.Leader())
}

// GetState 获取节点状态
func (n *HashicorpRaftNode) GetState() RaftState {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	state := RaftState{
		NodeID:      n.config.NodeID,
		Role:        "follower",
		Term:        0,
		LastIndex:   0,
		CommitIndex: 0,
		Peers:       []string{},
	}

	if n.raft == nil {
		return state
	}

	// 获取当前角色
	switch n.raft.State() {
	case raft.Leader:
		state.Role = "leader"
	case raft.Candidate:
		state.Role = "candidate"
	default:
		state.Role = "follower"
	}

	// 获取当前任期和最后日志索引
	// 使用GetConfiguration获取状态信息
	configFuture := n.raft.GetConfiguration()
	if err := configFuture.Error(); err == nil {
		// 无法直接获取Term，设置为0
		state.Term = 0
		state.LastIndex = n.raft.LastIndex()
		state.CommitIndex = n.raft.CommitIndex()
	}

	// 获取所有节点
	configFuture = n.raft.GetConfiguration()
	if err := configFuture.Error(); err == nil {
		for _, server := range configFuture.Configuration().Servers {
			state.Peers = append(state.Peers, string(server.ID))
		}
	}

	return state
}
