-- 创建文件元数据表
CREATE TABLE IF NOT EXISTS file_metadata (
    file_id VARCHAR(64) NOT NULL,
    shard_key INT UNSIGNED NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (file_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 创建块信息表
CREATE TABLE IF NOT EXISTS chunk_info (
    id BIGINT AUTO_INCREMENT,
    file_id VARCHAR(64) NOT NULL,
    chunk_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    offset BIGINT NOT NULL,
    size BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_file_id (file_id),
    KEY idx_chunk_id (chunk_id),
    KEY idx_node_id (node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 创建文件访问统计表
CREATE TABLE IF NOT EXISTS file_access_stats (
    file_id VARCHAR(64) NOT NULL,
    access_count BIGINT NOT NULL DEFAULT 0,
    last_access TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (file_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 创建节点状态表
CREATE TABLE IF NOT EXISTS node_status (
    node_id VARCHAR(64) NOT NULL,
    address VARCHAR(128) NOT NULL,
    disk_usage FLOAT NOT NULL DEFAULT 0,
    is_leader BOOLEAN NOT NULL DEFAULT FALSE,
    is_health BOOLEAN NOT NULL DEFAULT TRUE,
    last_heartbeat TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 创建系统配置表
CREATE TABLE IF NOT EXISTS system_config (
    config_key VARCHAR(64) NOT NULL,
    config_value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (config_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 插入初始系统配置
INSERT INTO system_config (config_key, config_value) VALUES 
('chunk_size', '4194304'),
('replication_factor', '3'),
('heartbeat_ttl', '3000'),
('disk_usage_threshold', '80');

-- 创建操作日志表
CREATE TABLE IF NOT EXISTS operation_logs (
    id BIGINT AUTO_INCREMENT,
    operation_type VARCHAR(32) NOT NULL,
    target_id VARCHAR(64) NOT NULL,
    node_id VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_target_id (target_id),
    KEY idx_node_id (node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4; 